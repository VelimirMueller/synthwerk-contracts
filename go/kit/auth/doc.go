// Package auth verifies the access tokens of the Synthwerk identity provider (Zitadel),
// and of other issuers a service trusts, in each service (D-20, D-23). No edge plugin checks JWTs.
//
//   - Verifier checks the signature against the issuer JWKS, then iss, aud, exp, nbf
//     and iat with a small leeway. Only RS256 and ES256 pass: never "none", never HS*.
//   - The JWKS is cached. An unknown kid refreshes it at once (rate limited); the cache
//     also refreshes after MaxAge. When a refresh fails, the last good key set stays.
//   - A ClaimsMapper turns the verified claims into a Principal (subject, org, roles,
//     scopes). ZitadelMapper is the default. Its claim names are in zitadel.go.
//   - MultiVerifier accepts more than one issuer: it picks the Verifier by the exact
//     "iss", and that Verifier checks the token with its own JWKS and audiences.
//     Principal.Issuer names the issuer; RequireIssuer keeps a route to some issuers.
//     llm uses it for the widget tokens of synthwerk-identity (synthwerk#30).
//   - Middleware puts the Principal in the request context. RequireRole, RequireOrg,
//     RequireScope and RequireMFA guard single routes. Refusals are RFC 9457
//     problem+json with a stable code: 401 unauthenticated, 403 forbidden or
//     mfa_required, 503 auth_unavailable.
//   - IDTokenVerifier checks an OIDC ID token with the same key set rules, for proof
//     of a recent sign-in (auth_time, amr, sid) that Zitadel access tokens do not
//     carry (synthwerk#41). Its audiences are client ids, not the project id.
//   - Package authtest serves a JWKS over httptest and mints tokens, so services test
//     without Zitadel.
//
// Wiring:
//
//	v, err := auth.NewVerifier(auth.Config{Issuer: iss, Audiences: []string{aud}})
//	api := http.NewServeMux()
//	api.Handle("GET /v1/orgs/{org}/members", auth.RequireOrg("org")(auth.RequireRole(auth.RoleAdmin)(h)))
//	mux := http.NewServeMux()
//	mux.Handle("/v1/", auth.Middleware(v, api)) // /healthz and /readyz stay open
//	healthHandler.Register(mux)
//
// Zitadel tokens (v4.20.0):
//
//   - "aud" is [client id, project id]. Set Audiences to the project id, so every app
//     of the project passes.
//   - The org id claim exists only when the client requests the scope
//     "urn:zitadel:iam:user:resourceowner". Without it ZitadelMapper refuses the
//     token (401), so a client that forgets the scope fails at once.
//   - Roles: Principal.Roles are the keys of the roles claim whose inner map holds
//     the token's org id. Grants in other orgs are ignored.
//   - No scope claim: Principal.Scopes stays empty and RequireScope refuses, until an
//     Action sets synthwerk:scope.
//   - RequireMFA reads only "amr" (evidence from the sign-in), never an enrolment
//     flag. Zitadel v4.20.0 access tokens carry "amr" only with the MFA gate of
//     synthwerk-identity on (synthwerk#94, synthwerk-infra ADR-0004): it refuses
//     every sign-in without a second factor and adds the "amr" of the gated sign-in.
//     Without the gate, Zitadel tokens have no "amr" and RequireMFA refuses them.
//
// RequireOrg reads a path value, so register it on the mux that matches the pattern.
// Add v.Ready as a readiness check ("jwks") so a service is not ready without keys.
package auth
