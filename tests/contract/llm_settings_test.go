package contract_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/VelimirMueller/synthwerk-contracts/go/llm"
)

// TestLLMAiSettings covers the AI settings routes of synthwerk#39: the spec examples
// parse strictly, stay inside their own bounds, and the generated client sends the
// right method, path and body.
func TestLLMAiSettings(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/llm/openapi.yaml")
	path := spec.Paths["/v1/ai-settings"]

	t.Run("AiSettings - each 200 example - decodes strictly and stays inside its bounds", func(t *testing.T) {
		for name, op := range map[string]specOp{"get": path.Get, "put": path.Put} {
			var s llm.AiSettings
			strict(t, toJSON(t, op.Responses["200"].Content["application/json"].Example), &s)
			if !slices.Contains(s.AllowedModels, s.Model) {
				t.Errorf("%s: model %q not in allowed_models %v", name, s.Model, s.AllowedModels)
			}
			for _, m := range s.AllowedModels {
				if !slices.Contains(s.Bounds.Models, m) {
					t.Errorf("%s: allowed model %q not in bounds.models %v", name, m, s.Bounds.Models)
				}
			}
			if s.MaxInputChars > s.Bounds.MaxInputChars || s.UserTokensPerDay > s.Bounds.UserTokensPerDay ||
				len(s.SystemPrompt) > int(s.Bounds.SystemPromptMaxChars) || s.Temperature == nil || *s.Temperature > s.Bounds.TemperatureMax {
				t.Errorf("%s: a value is outside the bounds: %+v", name, s)
			}
			if s.Version < 1 || s.UpdatedAt == nil {
				t.Errorf("%s: saved settings need a version and updated_at: %+v", name, s)
			}
		}
	})

	t.Run("UpdateAiSettings - spec request example - client sends PUT /v1/ai-settings with the body", func(t *testing.T) {
		raw, err := os.ReadFile("../../openapi/llm/openapi.yaml") // #nosec G304 -- spec path in the repo
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Paths map[string]struct {
				Put struct {
					RequestBody struct {
						Content map[string]struct {
							Example any `yaml:"example"`
						} `yaml:"content"`
					} `yaml:"requestBody"`
				} `yaml:"put"`
			} `yaml:"paths"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		var req llm.UpdateAiSettingsRequest
		strict(t, toJSON(t, doc.Paths["/v1/ai-settings"].Put.RequestBody.Content["application/json"].Example), &req)

		reply := toJSON(t, path.Put.Responses["200"].Content["application/json"].Example)
		var got string
		var sent llm.UpdateAiSettingsRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Method + " " + r.URL.Path
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sent)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(reply)
		}))
		t.Cleanup(srv.Close)
		c, err := llm.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.LlmUpdateAiSettingsWithResponse(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if got != "PUT /v1/ai-settings" || res.JSON200 == nil || res.JSON200.Version != req.Version+1 ||
			sent.Version != req.Version || sent.MaxInputChars != req.MaxInputChars {
			t.Fatalf("sent %q %+v, got %+v", got, sent, res.JSON200)
		}
	})

	problems := []struct {
		name, response, code string
		status               int
		check                func(c *llm.ClientWithResponses) (string, error)
	}{
		{"UpdateAiSettings - stale version - 409 version_conflict", "VersionConflict", "version_conflict", http.StatusConflict,
			func(c *llm.ClientWithResponses) (string, error) {
				res, err := c.LlmUpdateAiSettingsWithResponse(ctx, llm.UpdateAiSettingsRequest{Model: "default", AllowedModels: []string{"default"}, MaxInputChars: 1})
				if err != nil || res.ApplicationproblemJSON409 == nil {
					return "", err
				}
				return res.ApplicationproblemJSON409.Code, nil
			}},
		{"GetAiSettings - identity down - 503 settings_unavailable", "SettingsUnavailable", "settings_unavailable", http.StatusServiceUnavailable,
			func(c *llm.ClientWithResponses) (string, error) {
				res, err := c.LlmGetAiSettingsWithResponse(ctx)
				if err != nil || res.ApplicationproblemJSON503 == nil {
					return "", err
				}
				return res.ApplicationproblemJSON503.Code, nil
			}},
		{"GetAiSettings - member token - 403 forbidden", "Forbidden", "forbidden", http.StatusForbidden,
			func(c *llm.ClientWithResponses) (string, error) {
				res, err := c.LlmGetAiSettingsWithResponse(ctx)
				if err != nil || res.ApplicationproblemJSON403 == nil {
					return "", err
				}
				return res.ApplicationproblemJSON403.Code, nil
			}},
	}
	for _, p := range problems {
		t.Run(p.name, func(t *testing.T) {
			body := toJSON(t, spec.Components.Responses[p.response].Content["application/problem+json"].Example)
			var prob llm.Problem
			strict(t, body, &prob)
			if prob.Code != p.code || prob.Status != p.status {
				t.Fatalf("example: got %q %d", prob.Code, prob.Status)
			}
			c, err := llm.NewClientWithResponses(serveAs(t, p.status, "application/problem+json", body))
			if err != nil {
				t.Fatal(err)
			}
			code, err := p.check(c)
			if err != nil || code != p.code {
				t.Fatalf("got %q (%v), want %q", code, err, p.code)
			}
		})
	}
}
