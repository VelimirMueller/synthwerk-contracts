package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/cloudevents"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/health"
)

// healthSchema compiles one component schema of openapi/_shared/health.yaml.
func healthSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile("../../openapi/_shared/health.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("health.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("health.json#/components/schemas/" + name)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}
	return s
}

func validateBody(t *testing.T, s *jsonschema.Schema, body []byte) {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if err := s.Validate(v); err != nil {
		t.Fatalf("body does not match the schema: %v\n%s", err, body)
	}
}

func TestKitHealth(t *testing.T) {
	readiness := healthSchema(t, "Readiness")
	liveness := healthSchema(t, "Liveness")
	fail := func(context.Context) error { return errors.New("boom") }
	pass := func(context.Context) error { return nil }

	serve := func(t *testing.T, h *health.Handler, path string) (int, []byte) {
		t.Helper()
		mux := http.NewServeMux()
		h.Register(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return rec.Code, rec.Body.Bytes()
	}

	cases := []struct {
		name    string
		started bool
		checks  []health.Check
		code    int
	}{
		{"readyz - startup not complete - 503 matches schema", false, nil, 503},
		{"readyz - all checks pass - 200 matches schema", true, []health.Check{{Name: "postgres", Critical: true, Run: pass}}, 200},
		{"readyz - optional check fails - 200 matches schema", true, []health.Check{{Name: "cache", Run: fail}}, 200},
		{"readyz - critical check fails - 503 matches schema", true, []health.Check{{Name: "postgres", Critical: true, Run: fail, Detail: "postgres unreachable"}}, 503},
		{"readyz - no checks - 200 matches schema", true, nil, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := health.New("0.1.0", "3f9c2e1", tc.checks...)
			if err != nil {
				t.Fatal(err)
			}
			if tc.started {
				h.MarkStarted()
			}
			code, body := serve(t, h, "/readyz")
			if code != tc.code {
				t.Fatalf("status code %d, want %d", code, tc.code)
			}
			validateBody(t, readiness, body)
		})
	}

	t.Run("healthz - process runs - 200 matches schema", func(t *testing.T) {
		h, err := health.New("0.1.0", "3f9c2e1")
		if err != nil {
			t.Fatal(err)
		}
		code, body := serve(t, h, "/healthz")
		if code != http.StatusOK {
			t.Fatalf("status code %d, want 200", code)
		}
		validateBody(t, liveness, body)
	})
}

func TestKitCloudEvents(t *testing.T) {
	envelope := compile(t, filepath.Join(eventsDir, "envelope.schema.json"))

	t.Run("NATSSubject - every catalog type - equals catalog nats_subject", func(t *testing.T) {
		for _, e := range loadCatalog(t) {
			got, err := cloudevents.NATSSubject(e.Type)
			if err != nil {
				t.Fatalf("%s: %v", e.Type, err)
			}
			if got != e.NATSSubj {
				t.Errorf("%s: subject %q, catalog says %q", e.Type, got, e.NATSSubj)
			}
		}
	})

	t.Run("New - every catalog example - envelope validates", func(t *testing.T) {
		for _, e := range loadCatalog(t) {
			example := readJSON(t, filepath.Join(eventsDir, e.Example))
			src, err := cloudevents.Source(e.Producer, "ci")
			if err != nil {
				t.Fatal(err)
			}
			ev, err := cloudevents.New(context.Background(), e.Type, src, example["data"],
				cloudevents.WithSubject("x"), cloudevents.WithActor("system"), cloudevents.WithOrg("t_1"))
			if err != nil {
				t.Fatalf("%s: %v", e.Type, err)
			}
			raw, err := json.Marshal(ev)
			if err != nil {
				t.Fatal(err)
			}
			validateBody(t, envelope, raw)
		}
	})
}
