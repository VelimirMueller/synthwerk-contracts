package kit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/cloudevents"
)

const toolcallType = "dev.synthwerk.llm.toolcall.executed.v1"

func TestCloudEvents(t *testing.T) {
	ctx := context.Background()
	data := map[string]any{"ok": true}

	t.Run("New - valid input - UUIDv7 id and UTC time", func(t *testing.T) {
		e, err := cloudevents.New(ctx, toolcallType, "/synthwerk/llm/dev", data)
		if err != nil {
			t.Fatal(err)
		}
		id, err := uuid.Parse(e.ID)
		if err != nil || id.Version() != 7 {
			t.Fatalf("id %q is not a UUIDv7", e.ID)
		}
		if e.Time.Location().String() != "UTC" || e.SpecVersion != "1.0" || e.DataContentType != "application/json" {
			t.Fatalf("event %+v", e)
		}
	})

	t.Run("New - org set - orgid extension in JSON", func(t *testing.T) {
		e, err := cloudevents.New(ctx, toolcallType, "/synthwerk/llm/dev", data, cloudevents.WithOrg("org_7d3e9a"))
		if err != nil {
			t.Fatal(err)
		}
		if e.OrgID != "org_7d3e9a" {
			t.Fatalf("OrgID %q, want %q", e.OrgID, "org_7d3e9a")
		}
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m["orgid"] != "org_7d3e9a" || m["tenantid"] != nil {
			t.Fatalf("extensions %v, want orgid only", m)
		}
	})

	t.Run("New - span in context - traceparent of the span", func(t *testing.T) {
		tp := sdktrace.NewTracerProvider()
		t.Cleanup(func() { _ = tp.Shutdown(ctx) })
		sctx, span := tp.Tracer("test").Start(ctx, "op")
		defer span.End()
		e, err := cloudevents.New(sctx, toolcallType, "/synthwerk/llm/dev", data)
		if err != nil {
			t.Fatal(err)
		}
		carrier := propagation.MapCarrier{}
		propagation.TraceContext{}.Inject(sctx, carrier)
		if e.TraceParent == "" || e.TraceParent != carrier.Get("traceparent") {
			t.Fatalf("traceparent %q, want %q", e.TraceParent, carrier.Get("traceparent"))
		}
	})

	invalid := []struct {
		name string
		typ  string
		src  string
		data any
		opts []cloudevents.Option
	}{
		{"New - type without version - error", "dev.synthwerk.llm.toolcall.executed", "/synthwerk/llm/dev", data, nil},
		{"New - source with env ci - error", toolcallType, "/synthwerk/llm/ci", data, nil},
		{"New - actor is an email - error", toolcallType, "/synthwerk/llm/dev", data, []cloudevents.Option{cloudevents.WithActor("user:a@b.c")}},
		{"New - data is an array - error", toolcallType, "/synthwerk/llm/dev", []int{1}, nil},
		{"New - data is null - error", toolcallType, "/synthwerk/llm/dev", nil, nil},
		{"New - org with a space - error", toolcallType, "/synthwerk/llm/dev", data, []cloudevents.Option{cloudevents.WithOrg("a b")}},
		{"New - org longer than 64 characters - error", toolcallType, "/synthwerk/llm/dev", data, []cloudevents.Option{cloudevents.WithOrg(strings.Repeat("a", 65))}},
		// The deprecated tenantid path stays working (D-44).
		{"New - deprecated tenantid with a space - error", toolcallType, "/synthwerk/llm/dev", data, []cloudevents.Option{cloudevents.WithTenant("a b")}}, //nolint:staticcheck // proves the deprecated option still validates
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cloudevents.New(ctx, tc.typ, tc.src, tc.data, tc.opts...); err == nil {
				t.Fatal("no error")
			}
		})
	}

	t.Run("Source - env ci - local", func(t *testing.T) {
		s, err := cloudevents.Source("pulse", "ci")
		if err != nil || s != "/synthwerk/pulse/local" {
			t.Fatalf("got %q, %v", s, err)
		}
		if _, err := cloudevents.Source("Pulse", "dev"); err == nil {
			t.Fatal("no error for service Pulse")
		}
	})

	t.Run("NATSSubject - valid type - evt prefix", func(t *testing.T) {
		s, err := cloudevents.NATSSubject(toolcallType)
		if err != nil || s != "evt.llm.toolcall.executed.v1" {
			t.Fatalf("got %q, %v", s, err)
		}
		if _, err := cloudevents.NATSSubject("evt.llm.toolcall.executed.v1"); err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("no error for a subject as type: %v", err)
		}
	})
}
