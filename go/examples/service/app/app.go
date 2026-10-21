// Package app is the example Synthwerk Go service: it wires every kit package.
// Copy it as the start of a new service (pulse first, synthwerk#26).
//
// It owns one table, example_targets. POST /v1/targets/{name}/status stores a new
// status and, in the same transaction, the event dev.synthwerk.pulse.target.status_changed.v1
// in the outbox. The relay publishes it to JetStream.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/cloudevents"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/health"
	kitlog "github.com/VelimirMueller/synthwerk-contracts/go/kit/log"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/outbox"
)

// Service is the short service name: env prefix SYNTHWERK_EXAMPLE_, source /synthwerk/example/<env>.
const Service = "example"

// EventType is the event the example publishes.
const EventType = "dev.synthwerk.pulse.target.status_changed.v1"

const dataSchema = "https://schemas.synthwerk.dev/events/pulse/target.status_changed.v1.json"

// Schema creates the table of the example. A real service uses its migration tool.
const Schema = `
CREATE TABLE IF NOT EXISTS example_targets (
    name        text        PRIMARY KEY,
    status      text        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);`

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	checkRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	statuses = map[string]bool{"ok": true, "degraded": true, "down": true}
)

// App holds the dependencies of the service.
type App struct {
	DB      *pgxpool.Pool
	NC      *nats.Conn
	JS      jetstream.JetStream
	Log     *slog.Logger
	Health  *health.Handler
	Source  string
	Relay   *outbox.Relay
	handler http.Handler
}

// New wires health, the routes and the relay. Call Migrate before Run.
func New(db *pgxpool.Pool, nc *nats.Conn, logger *slog.Logger, version, commit, env string) (*App, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("app: jetstream: %w", err)
	}
	source, err := cloudevents.Source(Service, env)
	if err != nil {
		return nil, err
	}
	h, err := health.New(version, commit,
		health.Check{Name: "postgres", Critical: true, Run: db.Ping, Detail: "postgres unreachable"},
		health.Check{Name: "nats", Critical: true, Detail: "nats not connected", Run: func(context.Context) error {
			if !nc.IsConnected() {
				return errors.New("not connected")
			}
			return nil
		}},
	)
	if err != nil {
		return nil, err
	}
	relay, err := outbox.NewRelay(db, js, logger, outbox.Config{})
	if err != nil {
		return nil, err
	}
	a := &App{DB: db, NC: nc, JS: js, Log: logger, Health: h, Source: source, Relay: relay}

	mux := http.NewServeMux()
	h.Register(mux)
	mux.HandleFunc("POST /v1/targets/{name}/status", a.setStatus)
	// The log line sits inside the span; probes create no span.
	a.handler = otelhttp.NewHandler(kitlog.HTTP(logger, mux), Service,
		otelhttp.WithFilter(func(r *http.Request) bool { return r.URL.Path != "/healthz" && r.URL.Path != "/readyz" }))
	return a, nil
}

// Handler is the HTTP handler of the service.
func (a *App) Handler() http.Handler { return a.handler }

// Migrate creates the tables of the example and of the outbox.
func (a *App) Migrate(ctx context.Context) error {
	if _, err := a.DB.Exec(ctx, Schema); err != nil {
		return fmt.Errorf("app: migrate: %w", err)
	}
	return outbox.Migrate(ctx, a.DB)
}

type statusRequest struct {
	To     string `json:"to"`
	Checks []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"checks"`
}

type statusChanged struct {
	Target string        `json:"target"`
	From   string        `json:"from"`
	To     string        `json:"to"`
	Checks []checkResult `json:"checks"`
}

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (a *App) setStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	var req statusRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if !nameRe.MatchString(name) || dec.Decode(&req) != nil || !statuses[req.To] || len(req.Checks) > 32 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data := statusChanged{Target: name, To: req.To, Checks: make([]checkResult, 0, len(req.Checks))}
	for _, c := range req.Checks {
		if !checkRe.MatchString(c.Name) || !statuses[c.Status] {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		data.Checks = append(data.Checks, checkResult(c))
	}

	changed, err := a.store(ctx, data)
	if err != nil {
		a.Log.ErrorContext(ctx, "status not stored",
			slog.String("event.name", "example.status.failed"), slog.String("exception.message", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !changed {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// store writes the status and the event in one transaction. It writes nothing when
// the status did not change.
func (a *App) store(ctx context.Context, data statusChanged) (bool, error) {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	data.From = "unknown"
	err = tx.QueryRow(ctx, `SELECT status FROM example_targets WHERE name = $1 FOR UPDATE`, data.Target).Scan(&data.From)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if data.From == data.To {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO example_targets (name, status) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET status = EXCLUDED.status, updated_at = now()`,
		data.Target, data.To); err != nil {
		return false, err
	}
	ev, err := cloudevents.New(ctx, EventType, a.Source, data,
		cloudevents.WithSubject(data.Target),
		cloudevents.WithActor("service:"+Service),
		cloudevents.WithDataSchema(dataSchema))
	if err != nil {
		return false, err
	}
	if err := outbox.Enqueue(ctx, tx, ev); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// EnsureStream creates the stream EVT_PULSE when it is missing. Only for local runs
// and tests: in a deployment the infra module owns the stream config.
func EnsureStream(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.Stream(ctx, "EVT_PULSE")
	if err == nil {
		return nil
	}
	if !errors.Is(err, jetstream.ErrStreamNotFound) {
		return fmt.Errorf("app: stream: %w", err)
	}
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:       "EVT_PULSE",
		Subjects:   []string{"evt.pulse.>"},
		Duplicates: 2 * time.Minute, // >= relay restart time, see package outbox
	})
	if err != nil {
		return fmt.Errorf("app: create stream: %w", err)
	}
	return nil
}
