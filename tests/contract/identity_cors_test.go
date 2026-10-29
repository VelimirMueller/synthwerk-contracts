package contract_test

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestIdentityCORSHeaders: every answer whose CORS headers depend on the Origin
// documents Vary, and token answers and refusals document Cache-Control (synthwerk#38).
func TestIdentityCORSHeaders(t *testing.T) {
	raw, err := os.ReadFile("../../openapi/identity/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type op struct {
		Responses map[string]struct {
			Headers map[string]any `yaml:"headers"`
		} `yaml:"responses"`
	}
	var spec struct {
		Paths map[string]struct {
			Get  op `yaml:"get"`
			Post op `yaml:"post"`
		} `yaml:"paths"`
		Components struct {
			Responses map[string]struct {
				Headers map[string]any `yaml:"headers"`
			} `yaml:"responses"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		headers map[string]any
	}{
		{"getWidgetSettings 200", spec.Paths["/v1/apps/{appId}/widget-settings"].Get.Responses["200"].Headers},
		{"createWidgetToken 201", spec.Paths["/v1/apps/{appId}/widget-tokens"].Post.Responses["201"].Headers},
		{"OriginNotAllowed 403", spec.Components.Responses["OriginNotAllowed"].Headers},
	}
	for _, c := range cases {
		t.Run("CORS answer "+c.name+" - Vary and Cache-Control documented", func(t *testing.T) {
			if c.headers["Vary"] == nil || c.headers["Cache-Control"] == nil {
				t.Fatalf("headers %v", c.headers)
			}
		})
	}
}
