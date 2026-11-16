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
		{"listConversations", "200", spec.Paths["/v1/conversations"].Get},
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

// TestLlmOriginBinding: a widget token for origin A must not see a conversation made
// on origin B (synthwerk#54). Each conversation route states the rule, and list and
// read name the 404 answer for another origin.
func TestLlmOriginBinding(t *testing.T) {
	raw, err := os.ReadFile("../../openapi/llm/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type op struct {
		Description string `yaml:"description"`
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
		name string
		desc string
		want []string
	}{
		{"listConversations", spec.Paths["/v1/conversations"].Get.Description, []string{"Origin binding", "synthwerk:origin", "another origin", "never appear"}},
		{"createConversation", spec.Paths["/v1/conversations"].Post.Description, []string{"binds the conversation to its `synthwerk:origin`"}},
		{"getConversation", spec.Paths["/v1/conversations/{id}"].Get.Description, []string{"Origin binding", "another origin", "404"}},
		{"sendMessage", spec.Paths["/v1/conversations/{id}/messages"].Post.Description, []string{"Origin binding", "Another origin answers 404"}},
	}
	for _, c := range cases {
		t.Run("Origin binding "+c.name+" - widget token of origin A - never sees a conversation of origin B", func(t *testing.T) {
			for _, w := range c.want {
				if !strings.Contains(c.desc, w) {
					t.Fatalf("description lacks %q", w)
				}
			}
		})
	}
}
