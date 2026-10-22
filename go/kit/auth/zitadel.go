package auth

import (
	"errors"
	"slices"
	"strings"
)

// Zitadel claim names. All names in one place.
//
// Verified against Zitadel v4.20.0 on 2026-10-23 (spike SW-M2-0.5): ClaimOrgID,
// ClaimRoles and ClaimClientID, and the header typ "JWT". Access tokens have no "azp"
// and no "scope" claim; "aud" is [client id, project id].
const (
	// ClaimOrgID is the org of the user: a string. Zitadel sets it only when the
	// client requests the scope "urn:zitadel:iam:user:resourceowner".
	ClaimOrgID = "urn:zitadel:iam:user:resourceowner:id"
	// ClaimRoles is the project roles claim: {"<role>": {"<org id>": "<org domain>"}}.
	ClaimRoles = "urn:zitadel:iam:org:project:roles"
	// ClaimClientID is the client id of the app: studio, widget or sdk.
	ClaimClientID = "client_id"
	// ClaimScope is the AI scope (custom claim, set by an Action): "user" or a
	// space-separated list, or a list of strings. Not verified: no Zitadel token
	// carries it today, so Scopes stay empty.
	ClaimScope = "synthwerk:scope"
	// ClaimOrigin binds a widget token to one origin (#30). Not verified.
	ClaimOrigin = "synthwerk:origin"
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
	client, _ := claims[ClaimClientID].(string)
	if client == "" {
		client, _ = claims["azp"].(string) // ID tokens and other issuers
	}
	return &Principal{
		Subject:  sub,
		OrgID:    org,
		ClientID: client,
		Roles:    m.roles(claims[ClaimRoles], org),
		Scopes:   strs(claims[ClaimScope]),
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
