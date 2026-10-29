package contract_test

// strict (json.Unmarshal with DisallowUnknownFields) is defined in health_test.go
// in this package; `go test ./tests/contract` builds and passes.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/VelimirMueller/synthwerk-contracts/go/llm"
)

// specOp is the part of an operation the tests read: the response examples.
type specOp struct {
	Responses map[string]struct {
		Content map[string]struct {
			Example  any `yaml:"example"`
			Examples map[string]struct {
				Value any `yaml:"value"`
			} `yaml:"examples"`
		} `yaml:"content"`
	} `yaml:"responses"`
}

// specDoc is the part of a spec the tests read: operation and component response examples.
type specDoc struct {
	Paths map[string]struct {
		Get   specOp `yaml:"get"`
		Post  specOp `yaml:"post"`
		Patch specOp `yaml:"patch"`
	} `yaml:"paths"`
	Components struct {
		Responses map[string]struct {
			Content map[string]struct {
				Example  any `yaml:"example"`
				Examples map[string]struct {
					Value any `yaml:"value"`
				} `yaml:"examples"`
			} `yaml:"content"`
		} `yaml:"responses"`
	} `yaml:"components"`
}

func loadSpec(t *testing.T, path string) specDoc {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- spec path in the repo
	if err != nil {
		t.Fatal(err)
	}
	var spec specDoc
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func toJSON(t *testing.T, v any) []byte {
	t.Helper()
	if v == nil {
		t.Fatal("spec: example missing")
	}
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// serveAs answers every request with one status, content type and body.
func serveAs(t *testing.T, status int, contentType string, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestLLMContract(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/llm/openapi.yaml")
	send := spec.Paths["/v1/conversations/{id}/messages"].Post.Responses["200"].Content["text/event-stream"]

	t.Run("StreamEvent - each spec example - decodes to its event type", func(t *testing.T) {
		// Each event name maps to its generated type. A strict decode fails on an unknown field.
		want := map[string]any{
			"delta": &llm.DeltaEvent{},
			"usage": &llm.UsageEvent{},
			"done":  &llm.DoneEvent{},
			"error": &llm.ErrorEvent{},
		}
		if len(send.Examples) != len(want) {
			t.Fatalf("stream examples: got %d, want %d", len(send.Examples), len(want))
		}
		for name, typed := range want {
			ex, ok := send.Examples[name]
			if !ok {
				t.Fatalf("stream example %q missing", name)
			}
			body := toJSON(t, ex.Value)
			var ev llm.StreamEvent
			if err := json.Unmarshal(body, &ev); err != nil {
				t.Fatal(err)
			}
			if got, err := ev.Discriminator(); err != nil || got != name {
				t.Fatalf("discriminator: got %q (%v), want %q", got, err, name)
			}
			strict(t, body, typed)
		}
	})

	t.Run("StreamEvent - done example - message id and finish reason stop", func(t *testing.T) {
		var done llm.DoneEvent
		strict(t, toJSON(t, send.Examples["done"].Value), &done)
		if done.Data.FinishReason != llm.DoneFinishReasonStop || done.Data.MessageId.String() == "" {
			t.Fatalf("got %+v", done.Data)
		}
	})

	t.Run("SendMessage - no token - 401 problem with code unauthenticated", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["Unauthenticated"].Content["application/problem+json"].Example)
		strict(t, body, &llm.Problem{})

		client, err := llm.NewClientWithResponses(serveAs(t, http.StatusUnauthorized, "application/problem+json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.LlmSendMessageWithResponse(ctx, [16]byte{}, llm.SendMessageRequest{Content: "hi"})
		if err != nil {
			t.Fatal(err)
		}
		if res.ApplicationproblemJSON401 == nil || res.ApplicationproblemJSON401.Code != "unauthenticated" {
			t.Fatalf("got %+v", res.ApplicationproblemJSON401)
		}
	})

	t.Run("Problem - each 429 example - decodes strictly to a Problem with its code", func(t *testing.T) {
		examples := spec.Components.Responses["TooManyRequests"].Content["application/problem+json"].Examples
		for _, code := range []string{"too_many_streams", "quota_exhausted"} {
			ex, ok := examples[code]
			if !ok {
				t.Fatalf("429 example %q missing", code)
			}
			var p llm.Problem
			strict(t, toJSON(t, ex.Value), &p)
			if p.Code != code || p.Status != http.StatusTooManyRequests {
				t.Fatalf("example %q: got code %q status %d", code, p.Code, p.Status)
			}
		}
	})

	t.Run("Problem - quota_exhausted - names scope, unit, limit and reset", func(t *testing.T) {
		ex := spec.Components.Responses["TooManyRequests"].Content["application/problem+json"].Examples["quota_exhausted"]
		body := toJSON(t, ex.Value)
		var p llm.Problem
		strict(t, body, &p)
		q := p.Quota
		if q == nil {
			t.Fatal("quota_exhausted example has no quota member")
		}
		if !q.Scope.Valid() || !q.Unit.Valid() || q.Limit < 1 || q.ResetsAt.IsZero() {
			t.Fatalf("quota: got %+v", *q)
		}

		// The generated client hands the typed body to a caller that branches on code.
		client, err := llm.NewClientWithResponses(serveAs(t, http.StatusTooManyRequests, "application/problem+json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.LlmSendMessageWithResponse(ctx, [16]byte{}, llm.SendMessageRequest{Content: "hi"})
		if err != nil {
			t.Fatal(err)
		}
		got := res.ApplicationproblemJSON429
		if got == nil || got.Code != "quota_exhausted" || got.Quota == nil || got.Quota.Scope != llm.QuotaExceededScopeUser {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("Problem - quota member - only in the problem schema, optional", func(t *testing.T) {
		// Older clients decode a problem without quota; the member must stay optional.
		var p llm.Problem
		strict(t, []byte(`{"type":"https://schemas.synthwerk.dev/problems/too_many_streams","title":"Too many streams","status":429,"code":"too_many_streams"}`), &p)
		if p.Quota != nil {
			t.Fatalf("quota: got %+v, want nil", p.Quota)
		}
	})
}
