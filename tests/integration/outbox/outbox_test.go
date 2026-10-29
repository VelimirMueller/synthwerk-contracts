//go:build integration

package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/cloudevents"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/outbox"
	"github.com/VelimirMueller/synthwerk-contracts/tests/support/containers"
)

const (
	eventType = "dev.synthwerk.llm.toolcall.executed.v1"
	subject   = "evt.llm.toolcall.executed.v1"
	stream    = "EVT_LLM"
)

var env *containers.Env

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	var err error
	env, err = containers.Start(ctx)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "start containers:", err)
		os.Exit(1)
	}
	code := m.Run()
	env.Stop()
	os.Exit(code)
}

// setup returns a database with the outbox table and a fresh EVT_LLM stream.
func setup(t *testing.T) (*pgxpool.Pool, jetstream.JetStream) {
	t.Helper()
	ctx := t.Context()
	db := env.Database(t)
	if err := outbox.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE toolcalls (id text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.New(env.NATS(t))
	if err != nil {
		t.Fatal(err)
	}
	createStream(t, js)
	t.Cleanup(func() { _ = js.DeleteStream(context.Background(), stream) })
	return db, js
}

func createStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	if _, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name: stream, Subjects: []string{"evt.llm.>"}, Duplicates: 2 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
}

// write stores a data row and its event in one transaction and commits or rolls back.
func write(t *testing.T, db *pgxpool.Pool, commit bool) cloudevents.Event {
	t.Helper()
	ctx := t.Context()
	ev, err := cloudevents.New(ctx, eventType, "/synthwerk/llm/local",
		map[string]any{"tool": "pulse_get_health"}, cloudevents.WithActor("system"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO toolcalls (id) VALUES ($1)`, ev.ID); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(ctx, tx, ev); err != nil {
		t.Fatal(err)
	}
	if commit {
		err = tx.Commit(ctx)
	} else {
		err = tx.Rollback(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func relay(t *testing.T, db *pgxpool.Pool, js jetstream.JetStream, cfg outbox.Config) *outbox.Relay {
	t.Helper()
	r, err := outbox.NewRelay(db, js, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// consume reads every message of the stream with a fresh consumer, as a service would.
func consume(t *testing.T, js jetstream.JetStream) []jetstream.Msg {
	t.Helper()
	ctx := t.Context()
	c, err := js.OrderedConsumer(ctx, stream, jetstream.OrderedConsumerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := c.FetchNoWait(100)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []jetstream.Msg
	for m := range batch.Messages() {
		msgs = append(msgs, m)
	}
	if err := batch.Error(); err != nil {
		t.Fatal(err)
	}
	return msgs
}

func unpublished(t *testing.T, db *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := db.QueryRow(t.Context(),
		`SELECT count(*) FROM synthwerk_outbox WHERE published_at IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// age sets published_at and created_at of one row to now() minus the given ages.
// A negative published age leaves the row unpublished.
func age(t *testing.T, db *pgxpool.Pool, id string, created, published time.Duration) {
	t.Helper()
	var err error
	if published < 0 {
		_, err = db.Exec(t.Context(), `UPDATE synthwerk_outbox
			SET created_at = now() - make_interval(secs => $2), published_at = NULL WHERE id = $1`,
			id, created.Seconds())
	} else {
		_, err = db.Exec(t.Context(), `UPDATE synthwerk_outbox
			SET created_at = now() - make_interval(secs => $2),
			    published_at = now() - make_interval(secs => $3) WHERE id = $1`,
			id, created.Seconds(), published.Seconds())
	}
	if err != nil {
		t.Fatal(err)
	}
}

// ids returns the ids of every outbox row.
func ids(t *testing.T, db *pgxpool.Pool) map[string]bool {
	t.Helper()
	rows, err := db.Query(t.Context(), `SELECT id::text FROM synthwerk_outbox`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, id := range got {
		set[id] = true
	}
	return set
}

//nolint:contextcheck // the helpers take their context from t.
func TestOutbox(t *testing.T) {
	ctx := t.Context()

	t.Run("relay - transaction rolled back - no event published", func(t *testing.T) {
		db, js := setup(t)
		write(t, db, false)
		r := relay(t, db, js, outbox.Config{})

		n, err := r.RunOnce(ctx)
		if err != nil || n != 0 {
			t.Fatalf("RunOnce = %d, %v; want 0, nil", n, err)
		}
		if msgs := consume(t, js); len(msgs) != 0 {
			t.Fatalf("%d messages in the stream, want 0", len(msgs))
		}
		var rows int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM synthwerk_outbox`).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("%d outbox rows (%v), want 0", rows, err)
		}

		// Control: a committed transaction publishes, so the check above can fail.
		ev := write(t, db, true)
		if n, err := r.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
		}
		msgs := consume(t, js)
		if len(msgs) != 1 || msgs[0].Headers().Get(jetstream.MsgIDHeader) != ev.ID {
			t.Fatalf("%d messages, want 1 with Nats-Msg-Id %s", len(msgs), ev.ID)
		}
	})

	t.Run("relay - crash after publish, then restart - consumer gets the event once", func(t *testing.T) {
		db, js := setup(t)
		ev := write(t, db, true)

		// First relay: publishes, then "crashes" before it marks the row.
		crashed := relay(t, db, js, outbox.Config{BeforeMark: func(context.Context, string) error {
			return errors.New("crash")
		}})
		if _, err := crashed.RunOnce(ctx); err == nil {
			t.Fatal("RunOnce of the crashed relay returned no error")
		}
		if got := unpublished(t, db); got != 1 {
			t.Fatalf("%d unpublished rows after the crash, want 1", got)
		}
		if msgs := consume(t, js); len(msgs) != 1 {
			t.Fatalf("%d messages after the crash, want 1 (the publish happened)", len(msgs))
		}

		// Restart: a new relay runs its loop until the row is marked.
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- relay(t, db, js, outbox.Config{Interval: 20 * time.Millisecond}).Run(runCtx) }()
		deadline := time.Now().Add(10 * time.Second)
		for unpublished(t, db) != 0 {
			if time.Now().After(deadline) {
				stop()
				t.Fatal("the restarted relay did not mark the row in 10 s")
			}
			time.Sleep(20 * time.Millisecond)
		}
		stop()
		if err := <-done; err != nil {
			t.Fatalf("Run: %v", err)
		}

		msgs := consume(t, js)
		if len(msgs) != 1 {
			t.Fatalf("%d messages after the restart, want exactly 1", len(msgs))
		}
		var got cloudevents.Event
		if err := json.Unmarshal(msgs[0].Data(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != ev.ID || msgs[0].Headers().Get(jetstream.MsgIDHeader) != ev.ID {
			t.Fatalf("message id %s / header %s, want %s", got.ID, msgs[0].Headers().Get(jetstream.MsgIDHeader), ev.ID)
		}
		if err := got.Validate(); err != nil {
			t.Fatalf("published event is not valid: %v", err)
		}
	})

	t.Run("relay - no stream for the subject - row kept, published after the stream exists", func(t *testing.T) {
		db, js := setup(t)
		if err := js.DeleteStream(ctx, stream); err != nil {
			t.Fatal(err)
		}
		write(t, db, true)
		r := relay(t, db, js, outbox.Config{PublishTimeout: time.Second})

		if _, err := r.RunOnce(ctx); err == nil {
			t.Fatal("RunOnce without a stream returned no error")
		}
		var attempts int
		var lastError *string
		if err := db.QueryRow(ctx, `SELECT attempts, last_error FROM synthwerk_outbox`).Scan(&attempts, &lastError); err != nil {
			t.Fatal(err)
		}
		if attempts != 1 || lastError == nil || *lastError == "" || unpublished(t, db) != 1 {
			t.Fatalf("attempts %d, last_error %v; want 1 and a text, row unpublished", attempts, lastError)
		}

		createStream(t, js)
		if n, err := r.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
		}
		if msgs := consume(t, js); len(msgs) != 1 {
			t.Fatalf("%d messages, want 1", len(msgs))
		}
	})

	t.Run("relay - two relays at once - every event once", func(t *testing.T) {
		db, js := setup(t)
		const events = 50
		for range events {
			write(t, db, true)
		}
		a := relay(t, db, js, outbox.Config{BatchSize: 7})
		b := relay(t, db, js, outbox.Config{BatchSize: 7})
		errs := make(chan error, 2)
		for _, r := range []*outbox.Relay{a, b} {
			go func() {
				for {
					n, err := r.RunOnce(ctx)
					if err != nil || n == 0 {
						errs <- err
						return
					}
				}
			}()
		}
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		if got := unpublished(t, db); got != 0 {
			t.Fatalf("%d unpublished rows, want 0", got)
		}
		msgs := consume(t, js)
		seen := map[string]bool{}
		for _, m := range msgs {
			seen[m.Headers().Get(jetstream.MsgIDHeader)] = true
		}
		if len(msgs) != events || len(seen) != events {
			t.Fatalf("%d messages, %d distinct ids; want %d each", len(msgs), len(seen), events)
		}
	})

	t.Run("cleanup - published rows past the retention - deleted, unpublished rows kept", func(t *testing.T) {
		db, _ := setup(t)
		const day = 24 * time.Hour
		oldPublished := write(t, db, true)
		newPublished := write(t, db, true)
		oldUnpublished := write(t, db, true)
		newUnpublished := write(t, db, true)
		age(t, db, oldPublished.ID, 30*day, 8*day)
		age(t, db, newPublished.ID, 30*day, 2*time.Hour)
		age(t, db, oldUnpublished.ID, 30*day, -1)
		age(t, db, newUnpublished.ID, 0, -1)

		// Default retention (7 days): only the row published 8 days ago goes.
		n, err := outbox.Cleanup(ctx, db, outbox.DefaultRetention)
		if err != nil || n != 1 {
			t.Fatalf("Cleanup(7 days) = %d, %v; want 1, nil", n, err)
		}
		got := ids(t, db)
		if got[oldPublished.ID] || len(got) != 3 {
			t.Fatalf("rows after Cleanup(7 days): %v; want all but %s", got, oldPublished.ID)
		}

		// A shorter retention (1 h) also deletes the row published 2 h ago.
		if n, err := outbox.Cleanup(ctx, db, time.Hour); err != nil || n != 1 {
			t.Fatalf("Cleanup(1 h) = %d, %v; want 1, nil", n, err)
		}
		got = ids(t, db)
		if !got[oldUnpublished.ID] || !got[newUnpublished.ID] || len(got) != 2 {
			t.Fatalf("rows after Cleanup(1 h): %v; want only the 2 unpublished rows", got)
		}
		if got := unpublished(t, db); got != 2 {
			t.Fatalf("%d unpublished rows, want 2", got)
		}

		if _, err := outbox.Cleanup(ctx, db, 0); err == nil {
			t.Fatal("Cleanup(0) returned no error")
		}
	})

	t.Run("relay - retention set - Run deletes old published rows, publishes the rest", func(t *testing.T) {
		db, js := setup(t)
		old := write(t, db, true)
		age(t, db, old.ID, 2*time.Hour, time.Hour)
		pending := write(t, db, true)
		age(t, db, pending.ID, 2*time.Hour, -1)

		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- relay(t, db, js, outbox.Config{
				Interval: 20 * time.Millisecond, Retention: time.Minute, CleanupInterval: 20 * time.Millisecond,
			}).Run(runCtx)
		}()
		deadline := time.Now().Add(10 * time.Second)
		for {
			got := ids(t, db)
			if !got[old.ID] && unpublished(t, db) == 0 {
				break
			}
			if time.Now().After(deadline) {
				stop()
				t.Fatalf("rows after 10 s: %v; want %s deleted and every row published", got, old.ID)
			}
			time.Sleep(20 * time.Millisecond)
		}
		stop()
		if err := <-done; err != nil {
			t.Fatalf("Run: %v", err)
		}
		// The row published just now is younger than the retention: it stays.
		if got := ids(t, db); !got[pending.ID] || len(got) != 1 {
			t.Fatalf("rows: %v; want only %s", got, pending.ID)
		}
		if msgs := consume(t, js); len(msgs) != 1 {
			t.Fatalf("%d messages, want 1", len(msgs))
		}
	})
}
