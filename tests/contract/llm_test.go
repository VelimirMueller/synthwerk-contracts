package contract_test

// strict (json.Unmarshal with DisallowUnknownFields) is defined in health_test.go
// in this package; `go test ./tests/contract` builds and passes.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
				Example any `yaml:"example"`
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

	t.Run("Problem - quota_exhausted - reserved in the 429 response", func(t *testing.T) {
		raw, err := os.ReadFile("../../openapi/llm/openapi.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Components struct {
				Responses map[string]struct {
					Description string `yaml:"description"`
				} `yaml:"responses"`
			} `yaml:"components"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if d := doc.Components.Responses["TooManyRequests"].Description; !strings.Contains(d, "quota_exhausted") {
			t.Fatalf("429 response does not reserve quota_exhausted: %q", d)
		}
	})
}
