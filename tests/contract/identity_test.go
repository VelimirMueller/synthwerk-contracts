package contract_test

import (
	"context"
	"net/http"
	"testing"

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
}
