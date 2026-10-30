package contract_test

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestLlmCORSHeaders: the widget chat routes answer CORS in llm (infra#21). Each
// success answer documents Vary, and each route describes the preflight.
func TestLlmCORSHeaders(t *testing.T) {
	raw, err := os.ReadFile("../../openapi/llm/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type op struct {
		Description string `yaml:"description"`
		Responses   map[string]struct {
			Headers map[string]any `yaml:"headers"`
		} `yaml:"responses"`
	}
	var spec struct {
		Paths map[string]struct {
			Get  op `yaml:"get"`
			Post op `yaml:"post"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, status string
		op           op
	}{
		{"createConversation", "201", spec.Paths["/v1/conversations"].Post},
		{"getConversation", "200", spec.Paths["/v1/conversations/{id}"].Get},
		{"sendMessage", "200", spec.Paths["/v1/conversations/{id}/messages"].Post},
	}
	for _, c := range cases {
		t.Run("CORS answer "+c.name+" "+c.status+" - Vary documented, preflight described", func(t *testing.T) {
			if c.op.Responses[c.status].Headers["Vary"] == nil {
				t.Fatalf("headers %v", c.op.Responses[c.status].Headers)
			}
			for _, want := range []string{"OPTIONS", "Access-Control-Allow-Origin", "Vary: Origin", "Retry-After"} {
				if !strings.Contains(c.op.Description, want) {
					t.Fatalf("description lacks %q", want)
				}
			}
		})
	}
}
