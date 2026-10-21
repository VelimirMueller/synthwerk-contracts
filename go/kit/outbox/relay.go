package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ContentType is the NATS Content-Type header of a published event (structured mode).
const ContentType = "application/cloudevents+json"

const maxErrorLen = 512

// Beginner starts a database transaction. *pgxpool.Pool and *pgx.Conn implement it.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Publisher publishes one message to JetStream. jetstream.JetStream implements it.
type Publisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Config tunes the relay. The zero value works.
type Config struct {
	// Interval is the wait between two polls when the outbox is empty. Default 500 ms.
	Interval time.Duration
	// BatchSize is the max number of events per transaction. Default 100.
	BatchSize int
	// PublishTimeout limits one publish and its ack. Default 5 s.
	PublishTimeout time.Duration
	// BeforeMark runs after a publish and before the row is marked as published.
	// An error aborts the batch and rolls the marks back, like a crash at that point.
	// Tests use it to inject a crash. Leave it nil in services.
	BeforeMark func(ctx context.Context, eventID string) error
}

// Relay publishes outbox rows to JetStream. Several relays can run against one
// table: each locks its rows with FOR UPDATE SKIP LOCKED. Order is kept per relay
// (created_at, id), not across relays.
type Relay struct {
	db        Beginner
	js        Publisher
	log       *slog.Logger
	cfg       Config
	published metric.Int64Counter
	failed    metric.Int64Counter
}

// NewRelay returns a relay. logger may be nil.
func NewRelay(db Beginner, js Publisher, logger *slog.Logger, cfg Config) (*Relay, error) {
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = 5 * time.Second
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	meter := otel.Meter("github.com/VelimirMueller/synthwerk-contracts/go/kit/outbox")
	published, err := meter.Int64Counter("outbox.published",
		metric.WithDescription("Events the outbox relay published to JetStream."), metric.WithUnit("{event}"))
	if err != nil {
		return nil, fmt.Errorf("outbox: counter: %w", err)
	}
	failed, err := meter.Int64Counter("outbox.publish.errors",
		metric.WithDescription("Failed publish attempts of the outbox relay."), metric.WithUnit("{event}"))
	if err != nil {
		return nil, fmt.Errorf("outbox: counter: %w", err)
	}
	return &Relay{db: db, js: js, log: logger, cfg: cfg, published: published, failed: failed}, nil
}

// Run publishes until ctx ends. It returns nil when ctx ends. A failed batch is
// logged and tried again after Interval.
func (r *Relay) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		n, err := r.RunOnce(ctx)
		switch {
		case ctx.Err() != nil:
			// Stopped: a half-done batch rolled back and runs again after the restart.
		case err != nil:
			r.log.WarnContext(ctx, "outbox batch failed",
				slog.String("event.name", "outbox.batch.failed"),
				slog.String("exception.message", truncate(err.Error())))
		case n == r.cfg.BatchSize:
			continue // more rows wait
		}
		select {
		case <-ctx.Done():
		case <-time.After(r.cfg.Interval):
		}
	}
	return nil
}

type row struct {
	ID      string
	Subject string
	Event   []byte
}

// RunOnce publishes one batch in one transaction and returns the number of events
// it marked as published. A publish error stops the batch: the rows before it are
// marked, the failed row counts one more attempt, the rest waits for the next batch.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, nats_subject, event::text
		FROM synthwerk_outbox
		WHERE published_at IS NULL
		ORDER BY created_at, id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("outbox: select: %w", err)
	}
	batch, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	if err != nil {
		return 0, fmt.Errorf("outbox: read rows: %w", err)
	}
	if len(batch) == 0 {
		return 0, nil
	}

	published := 0
	var publishErr error
	for _, ev := range batch {
		if err := r.publish(ctx, ev); err != nil {
			publishErr = err
			r.failed.Add(ctx, 1, metric.WithAttributes(attribute.String("messaging.destination.name", ev.Subject)))
			r.log.WarnContext(ctx, "outbox publish failed",
				slog.String("event.name", "outbox.publish.failed"),
				slog.String("cloudevents.event_id", ev.ID),
				slog.String("messaging.destination.name", ev.Subject),
				slog.String("exception.message", truncate(err.Error())))
			if _, err := tx.Exec(ctx,
				`UPDATE synthwerk_outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`,
				ev.ID, truncate(err.Error())); err != nil {
				return 0, fmt.Errorf("outbox: record attempt %s: %w", ev.ID, err)
			}
			break
		}
		if r.cfg.BeforeMark != nil {
			if err := r.cfg.BeforeMark(ctx, ev.ID); err != nil {
				return 0, fmt.Errorf("outbox: before mark %s: %w", ev.ID, err)
			}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE synthwerk_outbox SET published_at = now(), attempts = attempts + 1, last_error = NULL WHERE id = $1`,
			ev.ID); err != nil {
			return 0, fmt.Errorf("outbox: mark %s: %w", ev.ID, err)
		}
		published++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("outbox: commit: %w", err)
	}
	if published > 0 {
		r.published.Add(ctx, int64(published))
		r.log.InfoContext(ctx, "outbox batch published",
			slog.String("event.name", "outbox.published"), slog.Int("synthwerk.outbox.count", published))
	}
	if publishErr != nil {
		return published, fmt.Errorf("outbox: publish: %w", publishErr)
	}
	return published, nil
}

func (r *Relay) publish(ctx context.Context, ev row) error {
	msg := nats.NewMsg(ev.Subject)
	msg.Data = ev.Event
	msg.Header.Set(jetstream.MsgIDHeader, ev.ID)
	msg.Header.Set("Content-Type", ContentType)

	pctx, cancel := context.WithTimeout(ctx, r.cfg.PublishTimeout)
	defer cancel()
	ack, err := r.js.PublishMsg(pctx, msg)
	if err != nil {
		return err
	}
	if ack == nil {
		return errors.New("no ack")
	}
	if ack.Duplicate {
		r.log.DebugContext(ctx, "outbox event was a duplicate",
			slog.String("event.name", "outbox.duplicate"),
			slog.String("cloudevents.event_id", ev.ID),
			slog.String("messaging.message.id", ev.ID))
	}
	return nil
}

// truncate cuts s to maxErrorLen bytes on a rune boundary (Postgres text needs valid UTF-8).
func truncate(s string) string {
	if len(s) <= maxErrorLen {
		return s
	}
	cut := maxErrorLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
