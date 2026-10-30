package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
)

// Problem codes of the refusals (RFC 9457 extension member "code").
const (
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
	CodeMFARequired     = "mfa_required"
	CodeUnavailable     = "auth_unavailable"
)

// ProblemContentType is the media type of every refusal body.
const ProblemContentType = "application/problem+json"

// Middleware verifies the Bearer token of each request and puts the Principal in the
// request context. No token or a refused token: 401. Key set not loadable: 503.
// The body is constant text: it never names the reason or echoes the token.
// v is a *Verifier (one issuer) or a *MultiVerifier (more issuers).
func Middleware(v TokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearer(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(w, http.StatusUnauthorized, CodeUnauthenticated, "A Bearer token is required.")
			return
		}
		p, err := v.Verify(r.Context(), raw)
		switch {
		case errors.Is(err, ErrUnavailable):
			writeProblem(w, http.StatusServiceUnavailable, CodeUnavailable, "Token keys are not available. Try again later.")
		case err != nil:
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeProblem(w, http.StatusUnauthorized, CodeUnauthenticated, "The token is not valid.")
		default:
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		}
	})
}

// RequireRole lets a request through when the Principal has at least one of roles.
// No Principal: 401. Else 403.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return require(func(_ *http.Request, p *Principal) bool {
		return slices.ContainsFunc(roles, p.HasRole)
	})
}

// RequireIssuer lets a request through when the token came from one of issuers
// (Principal.Issuer). No Principal: 401. Else 403. Use it to keep a route that a
// MultiVerifier guards to user tokens, for example to refuse widget tokens.
func RequireIssuer(issuers ...string) func(http.Handler) http.Handler {
	return require(func(_ *http.Request, p *Principal) bool {
		return p.Issuer != "" && slices.Contains(issuers, p.Issuer)
	})
}

// RequireScope lets a request through when the Principal has at least one of scopes.
// No Principal: 401. Else 403.
func RequireScope(scopes ...string) func(http.Handler) http.Handler {
	return require(func(_ *http.Request, p *Principal) bool {
		return slices.ContainsFunc(scopes, p.HasScope)
	})
}

// RequireMFA lets a request through when the sign-in used MFA (amr contains "mfa").
// No Principal: 401. Else 403 mfa_required. Use it on admin and platform routes.
//
// Only "amr" counts: it is evidence from the sign-in itself. A claim that says
// what the user has enrolled (for example "synthwerk:mfa") never counts.
//
// Zitadel v4.20.0 access tokens carry "amr" only when the MFA gate of
// synthwerk-identity is on (synthwerk#94, synthwerk-infra ADR-0004). The gate reads
// the session factors before Zitadel finishes a sign-in, refuses a session without a
// second factor, and its pre-access-token target copies the "amr" of the user's
// newest gated sign-in into the token. Without the gate, Zitadel tokens have no
// "amr" and RequireMFA refuses them (fail closed).
func RequireMFA() func(http.Handler) http.Handler {
	return requireCode(CodeMFARequired, "This request needs a sign-in with MFA.",
		func(_ *http.Request, p *Principal) bool { return p.HasMFA() })
}

// RequireOrg lets a request through when the path value param (for example "org" in
// /v1/orgs/{org}) equals the Principal's OrgID. No Principal: 401. Else 403.
// Register the guarded handler on the mux that matches the pattern, so the path value
// is set. Operator access to other orgs is a rule of the service, not of this helper.
func RequireOrg(param string) func(http.Handler) http.Handler {
	return require(func(r *http.Request, p *Principal) bool {
		org := r.PathValue(param)
		return org != "" && org == p.OrgID
	})
}

func require(allow func(*http.Request, *Principal) bool) func(http.Handler) http.Handler {
	return requireCode(CodeForbidden, "The token does not allow this request.", allow)
}

func requireCode(code, detail string, allow func(*http.Request, *Principal) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeProblem(w, http.StatusUnauthorized, CodeUnauthenticated, "A Bearer token is required.")
				return
			}
			if !allow(r, p) {
				writeProblem(w, http.StatusForbidden, code, detail)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearer returns the token of an "Authorization: Bearer <token>" header.
func bearer(r *http.Request) (string, bool) {
	scheme, tok, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	tok = strings.TrimSpace(tok)
	if !ok || !strings.EqualFold(scheme, "Bearer") || tok == "" || strings.ContainsAny(tok, " \t") {
		return "", false
	}
	return tok, true
}

// problem is an RFC 9457 body. Type about:blank: the title is the HTTP status text.
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{
		Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail, Code: code,
	})
}
