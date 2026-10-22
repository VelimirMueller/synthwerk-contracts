package auth

import (
	"context"
	"slices"
)

// Roles (D-44). owner, admin and member are org roles. operator is a platform role
// outside every org.
const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleMember   = "member"
	RoleOperator = "operator"
)

// Principal is the caller of a request, taken from a verified token.
type Principal struct {
	// Subject is the "sub" claim: the user or machine id. Owner of user data (D-30).
	Subject string
	// OrgID is the org the token acts in. Rows are scoped to it.
	OrgID string
	// Roles the caller has in OrgID, plus RoleOperator. Sorted, no duplicates.
	Roles []string
	// Scopes are the AI scopes (synthwerk:scope), not roles. Do not mix the two.
	Scopes []string
	// Raw holds every verified claim. Read other claims (azp, synthwerk:origin) here.
	Raw map[string]any
}

// HasRole reports whether p has role.
func (p *Principal) HasRole(role string) bool { return slices.Contains(p.Roles, role) }

// HasScope reports whether p has scope.
func (p *Principal) HasScope(scope string) bool { return slices.Contains(p.Scopes, scope) }

// IsOperator reports whether p has the platform role operator.
func (p *Principal) IsOperator() bool { return p.HasRole(RoleOperator) }

// ClaimsMapper turns the claims of a verified token into a Principal. It returns an
// error when a required claim is missing: Verify then refuses the token. Raw is set
// by Verify.
type ClaimsMapper interface {
	Map(claims map[string]any) (*Principal, error)
}

type principalKey struct{}

// WithPrincipal returns ctx with p. Middleware calls it; tests may too.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the Principal that Middleware put in ctx.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok && p != nil
}
