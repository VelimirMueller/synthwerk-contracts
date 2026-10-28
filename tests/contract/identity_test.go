package contract_test

// strict (json.Unmarshal with DisallowUnknownFields) is defined in health_test.go
// in this package; `go test ./tests/contract` builds and passes.

import (
	"context"
	"net/http"
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
