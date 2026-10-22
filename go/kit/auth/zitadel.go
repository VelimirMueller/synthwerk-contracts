package auth

import (
	"errors"
	"slices"
	"strings"
)

// Zitadel claim names (architecture §5.4). All names in one place.
//
// TODO(SW-M2-0.5): the names and shapes are not verified against a real Zitadel v4
// token yet. The spike captures one; change only this file when they differ.
const (
	// ClaimOrgID is the org of the user: a string.
	ClaimOrgID = "urn:zitadel:iam:user:resourceowner:id"
	// ClaimRoles is the project roles claim: {"<role>": {"<org id>": "<org domain>"}}.
	ClaimRoles = "urn:zitadel:iam:org:project:roles"
	// ClaimScope is the AI scope (custom, set by an Action): "user" or a
	// space-separated list, or a list of strings.
	ClaimScope = "synthwerk:scope"
	// ClaimOrigin binds a widget token to one origin (#30).
	ClaimOrigin = "synthwerk:origin"
	// ClaimAuthorizedParty is the client id: studio, widget or sdk.
	ClaimAuthorizedParty = "azp"
)

// ZitadelMapper maps Zitadel access token claims. A token without "sub" or org id is
// refused (spec #32 §3.8).
//
// Roles are kept per org: a role counts only when the roles claim grants it in the
// token's org. RoleOperator counts only when it is granted in PlatformOrgID.
type ZitadelMapper struct {
	// PlatformOrgID is the id of the Zitadel org "synthwerk" that grants operator.
	// Empty: no token gets RoleOperator.
	PlatformOrgID string
}

// Map implements ClaimsMapper.
func (m ZitadelMapper) Map(claims map[string]any) (*Principal, error) {
	sub, _ := claims["sub"].(string)
	org, _ := claims[ClaimOrgID].(string)
	var errs []error
	if sub == "" {
		errs = append(errs, errors.New("no sub"))
	}
	if org == "" {
		errs = append(errs, errors.New("no org id"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &Principal{
		Subject: sub,
		OrgID:   org,
		Roles:   m.roles(claims[ClaimRoles], org),
		Scopes:  strs(claims[ClaimScope]),
	}, nil
}

func (m ZitadelMapper) roles(claim any, org string) []string {
	grants, _ := claim.(map[string]any)
	roles := []string{}
	for role, v := range grants {
		orgs, _ := v.(map[string]any)
		in := org
		if role == RoleOperator {
			in = m.PlatformOrgID
		}
		if _, ok := orgs[in]; ok && in != "" {
			roles = append(roles, role)
		}
	}
	slices.Sort(roles)
	return roles
}

// strs reads a space-separated string or a list of strings.
func strs(v any) []string {
	var out []string
	switch v := v.(type) {
	case string:
		out = strings.Fields(v)
	case []any:
		for _, s := range v {
			if s, ok := s.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
