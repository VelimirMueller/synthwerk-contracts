package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VelimirMueller/synthwerk-contracts/go/identity"
	"github.com/VelimirMueller/synthwerk-contracts/go/llm"
)

// TestIdentityAccount covers the own-data routes of synthwerk#41: the spec examples
// parse strictly into the generated types and the generated clients read every
// documented answer.
func TestIdentityAccount(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/identity/openapi.yaml")
	llmSpec := loadSpec(t, "../../openapi/llm/openapi.yaml")
	problem := func(name string) []byte {
		return toJSON(t, spec.Components.Responses[name].Content["application/problem+json"].Example)
	}

	t.Run("ExportMe - spec example - identity rows and the llm part", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/me/export"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.UserExport{})

		c, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.IdentityExportMeWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		e := res.JSON200
		if e == nil || e.FormatVersion != identity.UserExportFormatVersionN1 || !e.Complete || e.Subject == "" ||
			len(e.Identity.Memberships) != 1 || e.Identity.Memberships[0].Role != identity.OrgRoleMember ||
			e.Identity.MfaEvidence == nil || len(e.Identity.MfaEvidence.Amr) != 3 ||
			len(e.Services) != 1 || e.Services[0].Service != "llm" || e.Services[0].Status != identity.ServiceExportStatusOk || e.Services[0].Data == nil {
			t.Fatalf("status %d, got %+v", res.StatusCode(), e)
		}
		// The llm part is an LlmExport of the llm spec.
		data, err := json.Marshal(e.Services[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		strict(t, data, &llm.LlmExport{})
	})

	t.Run("ExportMe - no token - 401 unauthenticated", func(t *testing.T) {
		c, err := identity.NewClientWithResponses(serveAs(t, http.StatusUnauthorized, "application/problem+json", problem("Unauthenticated")))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.IdentityExportMeWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.ApplicationproblemJSON401 == nil || res.ApplicationproblemJSON401.Code != "unauthenticated" {
			t.Fatalf("status %d, got %+v", res.StatusCode(), res.ApplicationproblemJSON401)
		}
	})

	t.Run("DeleteMe - spec example - accepted with the closed org", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/me"].Delete.Responses["202"].Content["application/json"].Example)
		strict(t, body, &identity.AccountDeletion{})

		c, err := identity.NewClientWithResponses(serveAs(t, http.StatusAccepted, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.IdentityDeleteMeWithResponse(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		d := res.JSON202
		if d == nil || d.Status != identity.AccountDeletionStatusAccepted || d.UserId == "" || len(d.DeletedOrgIds) != 1 || d.RequestedAt.IsZero() {
			t.Fatalf("status %d, got %+v", res.StatusCode(), d)
		}
	})

	t.Run("DeleteMe - ID token param - sent as X-Synthwerk-Id-Token", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("X-Synthwerk-Id-Token")
			w.WriteHeader(http.StatusAccepted)
		}))
		t.Cleanup(srv.Close)
		c, err := identity.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		idToken := "header.payload.signature" //nolint:gosec // G101: a placeholder, not a credential
		if _, err := c.IdentityDeleteMeWithResponse(ctx, &identity.IdentityDeleteMeParams{XSynthwerkIdToken: &idToken}); err != nil {
			t.Fatal(err)
		}
		if got != idToken {
			t.Fatalf("header %q, want the ID token", got)
		}
	})

	for _, tc := range []struct {
		name, response, code string
		status               int
		get                  func(*identity.IdentityDeleteMeResponse) *identity.Problem
	}{
		{"no recent MFA - 403 recent_mfa_required", "RecentMFARequired", "recent_mfa_required", http.StatusForbidden,
			func(r *identity.IdentityDeleteMeResponse) *identity.Problem { return r.ApplicationproblemJSON403 }},
		{"last owner of an org with members - 409 last_owner", "LastOwner", "last_owner", http.StatusConflict,
			func(r *identity.IdentityDeleteMeResponse) *identity.Problem { return r.ApplicationproblemJSON409 }},
		{"no token - 401 unauthenticated", "Unauthenticated", "unauthenticated", http.StatusUnauthorized,
			func(r *identity.IdentityDeleteMeResponse) *identity.Problem { return r.ApplicationproblemJSON401 }},
	} {
		t.Run("DeleteMe - "+tc.name, func(t *testing.T) {
			body := problem(tc.response)
			strict(t, body, &identity.Problem{})
			c, err := identity.NewClientWithResponses(serveAs(t, tc.status, "application/problem+json", body))
			if err != nil {
				t.Fatal(err)
			}
			res, err := c.IdentityDeleteMeWithResponse(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if p := tc.get(res); p == nil || p.Code != tc.code || p.Status != tc.status {
				t.Fatalf("status %d, got %+v; want %s", res.StatusCode(), p, tc.code)
			}
		})
	}

	t.Run("LLM ExportMe - spec example - conversations, messages with tokens, own quota rows", func(t *testing.T) {
		body := toJSON(t, llmSpec.Paths["/v1/me/export"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &llm.LlmExport{})

		c, err := llm.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.LlmExportMeWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		e := res.JSON200
		if e == nil || len(e.Conversations) != 1 || len(e.Conversations[0].Messages) != 2 ||
			e.Conversations[0].Messages[1].TokensOut == nil || *e.Conversations[0].Messages[1].TokensOut != 9 ||
			len(e.QuotaUsage) != 1 || e.QuotaUsage[0].Used != 21 || e.QuotaUsage[0].WindowStart.String() != "2026-10-23" {
			t.Fatalf("status %d, got %+v", res.StatusCode(), e)
		}
	})

	t.Run("LLM ExportMe - widget token - 403 forbidden", func(t *testing.T) {
		body := toJSON(t, llmSpec.Components.Responses["Forbidden"].Content["application/problem+json"].Example)
		c, err := llm.NewClientWithResponses(serveAs(t, http.StatusForbidden, "application/problem+json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.LlmExportMeWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.ApplicationproblemJSON403 == nil || res.ApplicationproblemJSON403.Code != "forbidden" {
			t.Fatalf("status %d, got %+v", res.StatusCode(), res.ApplicationproblemJSON403)
		}
	})
}
