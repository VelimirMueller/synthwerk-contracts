package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/VelimirMueller/synthwerk-contracts/go/health"
)

// example returns the response example of the spec for one path and status, as JSON.
func example(t *testing.T, path, status string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../openapi/_shared/health.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]struct {
			Get struct {
				Responses map[string]struct {
					Content map[string]struct {
						Example any `yaml:"example"`
					} `yaml:"content"`
				} `yaml:"responses"`
			} `yaml:"get"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	ex := spec.Paths[path].Get.Responses[status].Content["application/json"].Example
	if ex == nil {
		t.Fatalf("health.yaml: no example for %s %s", path, status)
	}
	body, err := json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// serve answers every request with one status and body, like a real service.
func serve(t *testing.T, status int, body []byte) *health.ClientWithResponses {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	client, err := health.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// strict decodes body into v and fails on a field the generated type does not know.
func strict(t *testing.T, body []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("spec example does not fit the generated type: %v", err)
	}
}

func TestHealthClient(t *testing.T) {
	ctx := context.Background()

	t.Run("GetLiveness - service runs - status ok", func(t *testing.T) {
		body := example(t, "/healthz", "200")
		strict(t, body, &health.Liveness{})

		res, err := serve(t, http.StatusOK, body).HealthGetLivenessWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 == nil || res.JSON200.Status != health.LivenessStatusOk {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("GetReadiness - one check degraded - 200 with status degraded", func(t *testing.T) {
		body := example(t, "/readyz", "200")
		strict(t, body, &health.Readiness{})

		res, err := serve(t, http.StatusOK, body).HealthGetReadinessWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := res.JSON200
		if got == nil || got.Status != health.ReadinessStatusDegraded || len(got.Checks) != 2 {
			t.Fatalf("got %+v", got)
		}
		if c := got.Checks[1]; c.Name != "signoz" || c.LatencyMs != 1500 || c.Detail == nil || *c.Detail != "export queue full" {
			t.Fatalf("check: %+v", c)
		}
	})

	t.Run("GetReadiness - critical check down - 503 with status down", func(t *testing.T) {
		body := example(t, "/readyz", "503")
		strict(t, body, &health.Readiness{})

		res, err := serve(t, http.StatusServiceUnavailable, body).HealthGetReadinessWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 != nil {
			t.Fatal("JSON200 set on a 503")
		}
		got := res.JSON503
		if got == nil || got.Status != health.ReadinessStatusDown || got.Checks[0].Name != "postgres" {
			t.Fatalf("got %+v", got)
		}
	})
}
