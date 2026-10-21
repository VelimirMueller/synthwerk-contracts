//go:build integration

package example_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/VelimirMueller/synthwerk-contracts/go/examples/service/app"
	"github.com/VelimirMueller/synthwerk-contracts/tests/support/containers"
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

func schema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../../events", path))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	s, err := c.Compile(abs)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

//nolint:contextcheck // the helpers take their context from t.
func TestExampleService(t *testing.T) {
	ctx := t.Context()
	db := env.Database(t)
	nc := env.NATS(t)
	a, err := app.New(db, nc, slog.New(slog.DiscardHandler), "0.1.0", "3f9c2e1", "ci")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureStream(ctx, a.JS); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.JS.DeleteStream(context.Background(), "EVT_PULSE") })
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	get := func(path string) int {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	post := func(target, body string) int {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/targets/"+target+"/status", strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}

	t.Run("readyz - before MarkStarted - 503", func(t *testing.T) {
		if code := get("/readyz"); code != http.StatusServiceUnavailable {
			t.Fatalf("status %d, want 503", code)
		}
	})

	a.Health.MarkStarted()

	t.Run("readyz - postgres and nats up - 200", func(t *testing.T) {
		if code := get("/readyz"); code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
		if code := get("/healthz"); code != http.StatusOK {
			t.Fatalf("healthz status %d, want 200", code)
		}
	})

	t.Run("set status - status changed - one valid event on EVT_PULSE", func(t *testing.T) {
		body := `{"to":"down","checks":[{"name":"postgres","status":"down"}]}`
		if code := post("identity", body); code != http.StatusAccepted {
			t.Fatalf("status %d, want 202", code)
		}
		if code := post("identity", body); code != http.StatusNoContent {
			t.Fatalf("same status again: %d, want 204", code)
		}
		if code := post("Identity", body); code != http.StatusBadRequest {
			t.Fatalf("bad target name: %d, want 400", code)
		}
		if n, err := a.Relay.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
		}

		c, err := a.JS.OrderedConsumer(ctx, "EVT_PULSE", jetstream.OrderedConsumerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		batch, err := c.FetchNoWait(10)
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
		if len(msgs) != 1 || msgs[0].Subject() != "evt.pulse.target.status_changed.v1" {
			t.Fatalf("%d messages, want 1 on evt.pulse.target.status_changed.v1", len(msgs))
		}
		event, err := jsonschema.UnmarshalJSON(bytes.NewReader(msgs[0].Data()))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema(t, "envelope.schema.json").Validate(event); err != nil {
			t.Fatalf("envelope: %v", err)
		}
		if err := schema(t, "pulse/target.status_changed.v1.schema.json").Validate(event.(map[string]any)["data"]); err != nil {
			t.Fatalf("data: %v", err)
		}
		var data struct{ From, To string }
		raw, _ := json.Marshal(event.(map[string]any)["data"])
		if err := json.Unmarshal(raw, &data); err != nil || data.From != "unknown" || data.To != "down" {
			t.Fatalf("data %+v, want from unknown to down", data)
		}
	})
}
