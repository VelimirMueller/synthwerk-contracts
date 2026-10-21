// Package outbox is the transactional outbox (D-07).
//
// A service writes its data row and the event row in one Postgres transaction with
// Enqueue. A rolled-back transaction leaves no event. The Relay reads unpublished rows,
// publishes each event to NATS JetStream with Nats-Msg-Id = the CloudEvents id, and
// marks the row as published in the same database transaction.
//
// Delivery: if the relay stops after the publish and before the commit, the row stays
// unpublished and the next run publishes it again. JetStream drops the second copy
// because the Nats-Msg-Id is the same. So a consumer sees the event once, as long as
// the relay restarts inside the duplicate window of the stream (JetStream default: 2 min).
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/cloudevents"
)

// Table is the name of the outbox table.
const Table = "synthwerk_outbox"

// Schema creates the outbox table. Run it with Migrate, or copy it into the
// migrations of the service.
const Schema = `
CREATE TABLE IF NOT EXISTS synthwerk_outbox (
    id            uuid        PRIMARY KEY,
    nats_subject  text        NOT NULL,
    event         jsonb       NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz,
    attempts      integer     NOT NULL DEFAULT 0,
    last_error    text
);
CREATE INDEX IF NOT EXISTS synthwerk_outbox_unpublished
    ON synthwerk_outbox (created_at, id) WHERE published_at IS NULL;
`

// Execer is the part of pgx.Tx, *pgxpool.Pool and *pgx.Conn that Enqueue needs.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Migrate creates the outbox table if it does not exist.
func Migrate(ctx context.Context, db Execer) error {
	if _, err := db.Exec(ctx, Schema); err != nil {
		return fmt.Errorf("outbox: migrate: %w", err)
	}
	return nil
}

// Enqueue stores the event in the outbox. Call it with the transaction that writes
// the data row: the event exists only if that transaction commits.
func Enqueue(ctx context.Context, tx Execer, e cloudevents.Event) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	subject, err := cloudevents.NATSSubject(e.Type)
	if err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("outbox: marshal event: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO synthwerk_outbox (id, nats_subject, event) VALUES ($1, $2, $3)`,
		e.ID, subject, raw,
	); err != nil {
		return fmt.Errorf("outbox: insert event %s: %w", e.ID, err)
	}
	return nil
}
