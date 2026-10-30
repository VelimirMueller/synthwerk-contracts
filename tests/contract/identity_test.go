package contract_test

// strict (json.Unmarshal with DisallowUnknownFields) is defined in health_test.go
// in this package; `go test ./tests/contract` builds and passes.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/VelimirMueller/synthwerk-contracts/go/identity"
)

func TestIdentityContract(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/identity/openapi.yaml")

	t.Run("GetMe - spec example - subject, org and roles", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/me"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.Me{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityGetMeWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		me := res.JSON200
		if me == nil || me.Subject == "" || me.Org.Slug != "synthwerk" || len(me.Roles) != 2 || me.Roles[1] != identity.RoleOperator {
			t.Fatalf("got %+v", me)
		}
	})

	t.Run("ListOrgs - spec example - one page of orgs", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.OrgPage{})
	})

	t.Run("GetOrg - not a member - 404 problem with code not_found", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["NotFound"].Content["application/problem+json"].Example)
		strict(t, body, &identity.Problem{})
	})

	t.Run("ListMembers - spec example - one page of members with org roles", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/members"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.MemberPage{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityListMembersWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), nil)
		if err != nil {
			t.Fatal(err)
		}
		page := res.JSON200
		if page == nil || len(page.Items) != 2 || page.Items[0].Role != identity.OrgRoleOwner {
			t.Fatalf("got %+v", page)
		}
	})

	t.Run("ChangeMemberRole - spec example - member with the new role", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/members/{userId}"].Patch.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.Member{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityChangeMemberRoleWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), "396370217707541255",
			identity.ChangeMemberRoleRequest{Role: identity.OrgRoleAdmin})
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 == nil || res.JSON200.Role != identity.OrgRoleAdmin {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("ChangeMemberRole - last owner - 409 problem with code last_owner", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["RoleConflict"].Content["application/problem+json"].Example)
		var p identity.Problem
		strict(t, body, &p)
		if p.Code != "last_owner" || p.Status != http.StatusConflict {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("InviteMember - spec example - 202 with the pending invitation", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/members"].Post.Responses["202"].Content["application/json"].Example)
		strict(t, body, &identity.Invitation{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusAccepted, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityInviteMemberWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"),
			identity.InviteMemberRequest{Email: "ada@example.com", Role: identity.OrgRoleMember})
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON202 == nil || res.JSON202.Role != identity.OrgRoleMember || res.JSON202.Email != "ada@example.com" {
			t.Fatalf("got %+v", res.JSON202)
		}
	})

	t.Run("ListInvitations - spec example - pending invitations", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/invitations"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.InvitationPage{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityListInvitationsWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 == nil || len(res.JSON200.Items) != 1 || res.JSON200.Items[0].InvitedBy == "" {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("InviteMember - too many invites - 429 problem with code rate_limited", func(t *testing.T) {
		r := spec.Components.Responses["RateLimited"]
		body := toJSON(t, r.Content["application/problem+json"].Example)
		var p identity.Problem
		strict(t, body, &p)
		if p.Code != "rate_limited" || p.Status != http.StatusTooManyRequests {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("InviteMember - email is a user already - 409 problem with code member_exists", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["MemberConflict"].Content["application/problem+json"].Example)
		var p identity.Problem
		strict(t, body, &p)
		if p.Code != "member_exists" || p.Status != http.StatusConflict {
			t.Fatalf("got %+v", p)
		}
	})

	for _, tc := range []struct {
		name, path string
		want       identity.MemberStatus
		call       func(c *identity.ClientWithResponses) (*identity.Member, error)
	}{
		{"DeactivateMember", "/v1/orgs/{orgId}/members/{userId}:deactivate", identity.MemberStatusDeactivated, func(c *identity.ClientWithResponses) (*identity.Member, error) {
			res, err := c.IdentityDeactivateMemberWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), "396370217707541255")
			if err != nil {
				return nil, err
			}
			return res.JSON200, nil
		}},
		{"ReactivateMember", "/v1/orgs/{orgId}/members/{userId}:reactivate", identity.MemberStatusActive, func(c *identity.ClientWithResponses) (*identity.Member, error) {
			res, err := c.IdentityReactivateMemberWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), "396370217707541255")
			if err != nil {
				return nil, err
			}
			return res.JSON200, nil
		}},
	} {
		t.Run(tc.name+" - spec example - member with the new status", func(t *testing.T) {
			body := toJSON(t, spec.Paths[tc.path].Post.Responses["200"].Content["application/json"].Example)
			strict(t, body, &identity.Member{})
			client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
			if err != nil {
				t.Fatal(err)
			}
			m, err := tc.call(client)
			if err != nil {
				t.Fatal(err)
			}
			if m == nil || m.Status != tc.want || !m.Status.Valid() {
				t.Fatalf("got %+v", m)
			}
		})
	}

	t.Run("ListAuditEvents - spec example - newest first, event data, next cursor", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/audit-events"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.AuditEventPage{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityListAuditEventsWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"), nil)
		if err != nil {
			t.Fatal(err)
		}
		page := res.JSON200
		if page == nil || len(page.Items) != 2 || page.NextCursor == nil {
			t.Fatalf("got %+v", page)
		}
		first := page.Items[0]
		if first.Type != "dev.synthwerk.identity.member.role_changed.v1" || first.Data["new_role"] != "admin" || !first.Time.After(page.Items[1].Time) {
			t.Fatalf("got %+v", first)
		}
	})

	t.Run("MemberStatus - enum - active and deactivated only", func(t *testing.T) {
		if !identity.MemberStatusActive.Valid() || !identity.MemberStatusDeactivated.Valid() || identity.MemberStatus("pending").Valid() {
			t.Fatal("MemberStatus enum")
		}
	})

	t.Run("ListApps - spec example - one page of apps with origins", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/apps"].Get.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.AppPage{})
	})

	t.Run("CreateApp - spec example - app with the canonical allow-list", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/apps"].Post.Responses["201"].Content["application/json"].Example)
		strict(t, body, &identity.App{})

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusCreated, "application/json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityCreateAppWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"),
			identity.CreateAppRequest{Name: "Shop website", AllowedOrigins: []identity.AppOrigin{"https://shop.example"}})
		if err != nil {
			t.Fatal(err)
		}
		app := res.JSON201
		if app == nil || app.Name != "Shop website" || len(app.AllowedOrigins) != 2 || app.AllowedOrigins[0] != "https://shop.example" {
			t.Fatalf("got %+v", app)
		}
	})

	t.Run("UpdateApp - spec example - app with the new allow-list", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/apps/{appId}"].Patch.Responses["200"].Content["application/json"].Example)
		strict(t, body, &identity.App{})
	})

	t.Run("CreateWidgetToken - spec example - bearer token, 15 minutes or less", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/apps/{appId}/widget-tokens"].Post.Responses["201"].Content["application/json"].Example)
		strict(t, body, &identity.WidgetToken{})

		var gotOrigin string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotOrigin = r.Header.Get("Origin")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		client, err := identity.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityCreateWidgetTokenWithResponse(ctx, uuid.MustParse("019a3b10-7c2d-7e4f-8a5b-6c7d8e9f0a1b"),
			&identity.IdentityCreateWidgetTokenParams{Origin: "https://shop.example"})
		if err != nil {
			t.Fatal(err)
		}
		tok := res.JSON201
		if gotOrigin != "https://shop.example" {
			t.Fatalf("Origin header = %q", gotOrigin)
		}
		if tok == nil || tok.TokenType != identity.WidgetTokenTokenTypeBearer || tok.ExpiresIn < 1 || tok.ExpiresIn > 900 || tok.Origin != "https://shop.example" {
			t.Fatalf("got %+v", tok)
		}
	})

	t.Run("CreateWidgetToken - origin not listed - 403 problem with code origin_not_allowed", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["OriginNotAllowed"].Content["application/problem+json"].Example)
		var p identity.Problem
		strict(t, body, &p)
		if p.Code != "origin_not_allowed" || p.Status != http.StatusForbidden {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("CreateApp - org at its app limit - 403 problem with code app_limit_reached", func(t *testing.T) {
		body := toJSON(t, spec.Components.Responses["CreateAppForbidden"].Content["application/problem+json"].Example)
		var p identity.Problem
		strict(t, body, &p)
		if p.Code != "app_limit_reached" || p.Status != http.StatusForbidden {
			t.Fatalf("got %+v", p)
		}

		client, err := identity.NewClientWithResponses(serveAs(t, http.StatusForbidden, "application/problem+json", body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityCreateAppWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"),
			identity.CreateAppRequest{Name: "Fourth app", AllowedOrigins: []identity.AppOrigin{"https://shop.example"}})
		if err != nil {
			t.Fatal(err)
		}
		if res.ApplicationproblemJSON403 == nil || res.ApplicationproblemJSON403.Code != "app_limit_reached" {
			t.Fatalf("status %d, got %+v", res.StatusCode(), res.ApplicationproblemJSON403)
		}
	})

	t.Run("UpdateApp - widget settings - the client sends them, the answer carries them", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/orgs/{orgId}/apps/{appId}"].Patch.Responses["200"].Content["application/json"].Example)
		var sent map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&sent)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		client, err := identity.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		settings := identity.WidgetSettings{
			Position: identity.WidgetSettingsPositionBottomLeft, Theme: identity.WidgetSettingsThemeDefault,
			Mode: identity.WidgetSettingsModeDark, Lang: identity.WidgetSettingsLangDe,
			Accent: identity.WidgetSettingsAccentLab, Greeting: "Hallo! Wie kann ich helfen?",
		}
		res, err := client.IdentityUpdateAppWithResponse(ctx, uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60"),
			uuid.MustParse("019a3b10-7c2d-7e4f-8a5b-6c7d8e9f0a1b"), identity.UpdateAppRequest{WidgetSettings: &settings})
		if err != nil {
			t.Fatal(err)
		}
		ws, ok := sent["widget_settings"].(map[string]any)
		if !ok || ws["position"] != "bottom-left" || ws["accent"] != "lab" || ws["greeting"] != "Hallo! Wie kann ich helfen?" {
			t.Fatalf("sent %+v", sent)
		}
		if _, ok := sent["name"]; ok {
			t.Fatalf("absent name was sent: %+v", sent)
		}
		if res.JSON200 == nil || res.JSON200.WidgetSettings != settings {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("GetWidgetSettings - spec example - settings only, Origin sent, no token", func(t *testing.T) {
		body := toJSON(t, spec.Paths["/v1/apps/{appId}/widget-settings"].Get.Responses["200"].Content["application/json"].Example)
		var ws identity.WidgetSettings
		strict(t, body, &ws)

		var gotOrigin, gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotOrigin, gotAuth = r.Header.Get("Origin"), r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		client, err := identity.NewClientWithResponses(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.IdentityGetWidgetSettingsWithResponse(ctx, uuid.MustParse("019a3b10-7c2d-7e4f-8a5b-6c7d8e9f0a1b"),
			&identity.IdentityGetWidgetSettingsParams{Origin: "https://shop.example"})
		if err != nil {
			t.Fatal(err)
		}
		if gotOrigin != "https://shop.example" || gotAuth != "" {
			t.Fatalf("Origin %q, Authorization %q", gotOrigin, gotAuth)
		}
		if res.JSON200 == nil || *res.JSON200 != ws || !ws.Position.Valid() || !ws.Accent.Valid() {
			t.Fatalf("got %+v", res.JSON200)
		}
	})

	t.Run("WidgetSettings - enums - only token themes and accent groups", func(t *testing.T) {
		if identity.WidgetSettingsAccent("#ff0000").Valid() || identity.WidgetSettingsTheme("neon").Valid() {
			t.Fatal("a free colour or theme is valid")
		}
		for _, a := range []identity.WidgetSettingsAccent{identity.WidgetSettingsAccentFlagship, identity.WidgetSettingsAccentLab,
			identity.WidgetSettingsAccentOps, identity.WidgetSettingsAccentVenture} {
			if !a.Valid() {
				t.Fatalf("%s not valid", a)
			}
		}
	})

	t.Run("OrgRole - enum - operator is never an org role", func(t *testing.T) {
		for _, r := range []identity.OrgRole{identity.OrgRoleOwner, identity.OrgRoleAdmin, identity.OrgRoleMember} {
			if !r.Valid() {
				t.Fatalf("%s not valid", r)
			}
		}
		if identity.OrgRole("operator").Valid() {
			t.Fatal("operator is a valid OrgRole")
		}
	})
}
