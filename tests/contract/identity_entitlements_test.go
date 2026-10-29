package contract_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/VelimirMueller/synthwerk-contracts/go/identity"
)

// TestIdentityEntitlements covers the entitlement routes of synthwerk#31: the spec
// examples parse strictly and the generated client sends the right method and path.
func TestIdentityEntitlements(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, "../../openapi/identity/openapi.yaml")
	org := uuid.MustParse("0199f3a0-5b6c-7d8e-9f0a-1b2c3d4e5f60")

	routes := []struct {
		name, path, want string
		call             func(c *identity.ClientWithResponses) (*identity.Entitlements, *identity.NotFound, int, error)
	}{
		{"GetMyEntitlements", "/v1/me/entitlements", "GET /v1/me/entitlements",
			func(c *identity.ClientWithResponses) (*identity.Entitlements, *identity.NotFound, int, error) {
				res, err := c.IdentityGetMyEntitlementsWithResponse(ctx)
				if err != nil {
					return nil, nil, 0, err
				}
				return res.JSON200, res.ApplicationproblemJSON404, res.StatusCode(), nil
			}},
		{"GetOrgEntitlements", "/v1/orgs/{orgId}/entitlements", "GET /v1/orgs/" + org.String() + "/entitlements",
			func(c *identity.ClientWithResponses) (*identity.Entitlements, *identity.NotFound, int, error) {
				res, err := c.IdentityGetOrgEntitlementsWithResponse(ctx, org)
				if err != nil {
					return nil, nil, 0, err
				}
				return res.JSON200, res.ApplicationproblemJSON404, res.StatusCode(), nil
			}},
	}

	for _, r := range routes {
		t.Run(r.name+" - spec example - free plan with its limits", func(t *testing.T) {
			body := toJSON(t, spec.Paths[r.path].Get.Responses["200"].Content["application/json"].Example)
			strict(t, body, &identity.Entitlements{})

			c, err := identity.NewClientWithResponses(serveAs(t, http.StatusOK, "application/json", body))
			if err != nil {
				t.Fatal(err)
			}
			e, _, status, err := r.call(c)
			if err != nil {
				t.Fatal(err)
			}
			if status != http.StatusOK || e == nil || e.Plan != "free" || e.OrgId != org || e.Version != 1 ||
				e.Limits.LlmTokensPerUserPerDay != 50000 || e.Limits.LlmTokensPerOrgPerDay != 500000 || e.Limits.AppsPerOrg != 3 {
				t.Fatalf("status %d, got %+v", status, e)
			}
		})

		t.Run(r.name+" - unknown org - 404 problem, method and path match the spec", func(t *testing.T) {
			problem := toJSON(t, spec.Components.Responses["NotFound"].Content["application/problem+json"].Example)
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				got = req.Method + " " + req.URL.Path
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(problem)
			}))
			t.Cleanup(srv.Close)
			c, err := identity.NewClientWithResponses(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			e, nf, status, err := r.call(c)
			if err != nil {
				t.Fatal(err)
			}
			if got != r.want || status != http.StatusNotFound || e != nil || nf == nil || nf.Code != "not_found" {
				t.Fatalf("sent %q, status %d, body %+v, problem %+v; want %q and 404 not_found", got, status, e, nf, r.want)
			}
		})
	}

}
