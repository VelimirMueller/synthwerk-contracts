package kit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	api "github.com/VelimirMueller/synthwerk-contracts/go/health"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/health"
)

func readyz(t *testing.T, h *health.Handler) (int, api.Readiness) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Readiness(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil))
	var body api.Readiness
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func TestHealth(t *testing.T) {
	pass := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("dial tcp 10.0.0.1:5432: secret detail") }

	t.Run("readyz - startup not complete - 503 down", func(t *testing.T) {
		h, err := health.New("0.1.0", "3f9c2e1", health.Check{Name: "postgres", Critical: true, Run: pass})
		if err != nil {
			t.Fatal(err)
		}
		code, body := readyz(t, h)
		if code != http.StatusServiceUnavailable || body.Status != api.ReadinessStatusDown {
			t.Fatalf("got %d %s, want 503 down", code, body.Status)
		}
	})

	t.Run("readyz - optional dependency down - 200 degraded", func(t *testing.T) {
		h, err := health.New("0.1.0", "3f9c2e1",
			health.Check{Name: "postgres", Critical: true, Run: pass},
			health.Check{Name: "cache", Run: fail})
		if err != nil {
			t.Fatal(err)
		}
		h.MarkStarted()
		code, body := readyz(t, h)
		if code != http.StatusOK || body.Status != api.ReadinessStatusDegraded {
			t.Fatalf("got %d %s, want 200 degraded", code, body.Status)
		}
	})

	t.Run("readyz - critical dependency down - 503 down without the error text", func(t *testing.T) {
		h, err := health.New("0.1.0", "3f9c2e1",
			health.Check{Name: "postgres", Critical: true, Run: fail, Detail: "postgres unreachable"})
		if err != nil {
			t.Fatal(err)
		}
		h.MarkStarted()
		code, body := readyz(t, h)
		if code != http.StatusServiceUnavailable || body.Status != api.ReadinessStatusDown {
			t.Fatalf("got %d %s, want 503 down", code, body.Status)
		}
		if d := body.Checks[0].Detail; d == nil || *d != "postgres unreachable" {
			t.Fatalf("detail %v, want the constant text", d)
		}
	})

	t.Run("readyz - check hangs - answers within the budget with timeout", func(t *testing.T) {
		hang := func(ctx context.Context) error { <-ctx.Done(); time.Sleep(2 * time.Second); return ctx.Err() }
		h, err := health.New("0.1.0", "3f9c2e1",
			health.Check{Name: "slow", Critical: true, Timeout: 50 * time.Millisecond, Run: hang})
		if err != nil {
			t.Fatal(err)
		}
		h.MarkStarted()
		start := time.Now()
		code, body := readyz(t, h)
		if took := time.Since(start); took > time.Second {
			t.Fatalf("readyz took %s, want < 1 s", took)
		}
		if code != http.StatusServiceUnavailable || body.Checks[0].Detail == nil || *body.Checks[0].Detail != "timeout" {
			t.Fatalf("got %d %+v, want 503 with detail timeout", code, body.Checks[0])
		}
	})

	t.Run("New - invalid commit or check name - error", func(t *testing.T) {
		if _, err := health.New("0.1.0", "unknown"); err == nil {
			t.Error("no error for commit unknown")
		}
		if _, err := health.New("0.1.0", "3f9c2e1", health.Check{Name: "Postgres", Run: pass}); err == nil {
			t.Error("no error for check name Postgres")
		}
		if _, err := health.New("0.1.0", "3f9c2e1", health.Check{Name: "pg", Run: pass}, health.Check{Name: "pg", Run: pass}); err == nil {
			t.Error("no error for a duplicate check name")
		}
	})
}
