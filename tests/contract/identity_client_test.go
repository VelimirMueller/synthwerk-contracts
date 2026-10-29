package contract_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/VelimirMueller/synthwerk-contracts/go/identity"
)

// TestIdentityClientRoutes calls every operation of the generated identity client
// and checks the method and path it sends, and that a problem answer parses.
func TestIdentityClientRoutes(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/identity/openapi.yaml")
	problem := toJSON(t, spec.Components.Responses["NotFound"].Content["application/problem+json"].Example)
	org := uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60")
	app := uuid.MustParse("019a3b10-7c2d-7e4f-8a5b-6c7d8e9f0a1b")
	limit, cursor := 10, "abc"
	name := "Shop"
	origins := []identity.AppOrigin{"https://shop.example"}

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(problem)
	}))
	t.Cleanup(srv.Close)
	c, err := identity.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	type result interface{ StatusCode() int }
	cases := []struct {
		name, want string
		call       func() (result, error)
	}{
		{"getMe", "GET /v1/me", func() (result, error) { return c.IdentityGetMeWithResponse(ctx) }},
		{"listOrgs", "GET /v1/orgs", func() (result, error) {
			return c.IdentityListOrgsWithResponse(ctx, &identity.IdentityListOrgsParams{Limit: &limit, Cursor: &cursor})
		}},
		{"createOrg", "POST /v1/orgs", func() (result, error) {
			return c.IdentityCreateOrgWithResponse(ctx, identity.CreateOrgRequest{Name: "Acme", Slug: "acme"})
		}},
		{"getOrg", "GET /v1/orgs/" + org.String(), func() (result, error) { return c.IdentityGetOrgWithResponse(ctx, org) }},
		{"listMembers", "GET /v1/orgs/" + org.String() + "/members", func() (result, error) {
			return c.IdentityListMembersWithResponse(ctx, org, &identity.IdentityListMembersParams{Limit: &limit})
		}},
		{"listApps", "GET /v1/orgs/" + org.String() + "/apps", func() (result, error) {
			return c.IdentityListAppsWithResponse(ctx, org, &identity.IdentityListAppsParams{Limit: &limit, Cursor: &cursor})
		}},
		{"createApp", "POST /v1/orgs/" + org.String() + "/apps", func() (result, error) {
			return c.IdentityCreateAppWithResponse(ctx, org, identity.CreateAppRequest{Name: name, AllowedOrigins: origins})
		}},
		{"getApp", "GET /v1/orgs/" + org.String() + "/apps/" + app.String(), func() (result, error) {
			return c.IdentityGetAppWithResponse(ctx, org, app)
		}},
		{"updateApp", "PATCH /v1/orgs/" + org.String() + "/apps/" + app.String(), func() (result, error) {
			return c.IdentityUpdateAppWithResponse(ctx, org, app, identity.UpdateAppRequest{Name: &name, AllowedOrigins: &origins})
		}},
		{"deleteApp", "DELETE /v1/orgs/" + org.String() + "/apps/" + app.String(), func() (result, error) {
			return c.IdentityDeleteAppWithResponse(ctx, org, app)
		}},
		{"getWidgetSettings", "GET /v1/apps/" + app.String() + "/widget-settings", func() (result, error) {
			return c.IdentityGetWidgetSettingsWithResponse(ctx, app, &identity.IdentityGetWidgetSettingsParams{Origin: "https://shop.example"})
		}},
		{"createWidgetToken", "POST /v1/apps/" + app.String() + "/widget-tokens", func() (result, error) {
			return c.IdentityCreateWidgetTokenWithResponse(ctx, app, &identity.IdentityCreateWidgetTokenParams{Origin: "https://shop.example"})
		}},
	}
	for _, tc := range cases {
		t.Run("Client "+tc.name+" - 404 problem - method and path match the spec", func(t *testing.T) {
			res, err := tc.call()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || res.StatusCode() != http.StatusNotFound {
				t.Fatalf("sent %q, status %d; want %q", got, res.StatusCode(), tc.want)
			}
		})
	}
}
