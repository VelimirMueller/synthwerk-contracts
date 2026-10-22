// Package auth verifies the access tokens of the Synthwerk identity provider (Zitadel)
// in each service (D-20, D-23). No edge plugin checks JWTs.
//
//   - Verifier checks the signature against the issuer JWKS, then iss, aud, exp, nbf
//     and iat with a small leeway. Only RS256 and ES256 pass: never "none", never HS*.
//   - The JWKS is cached. An unknown kid refreshes it at once (rate limited); the cache
//     also refreshes after MaxAge. When a refresh fails, the last good key set stays.
//   - A ClaimsMapper turns the verified claims into a Principal (subject, org, roles,
//     scopes). ZitadelMapper is the default. Its claim names are in zitadel.go.
//   - Middleware puts the Principal in the request context. RequireRole, RequireOrg
//     and RequireScope guard single routes. Refusals are RFC 9457 problem+json with a
//     stable code: 401 unauthenticated, 403 forbidden, 503 auth_unavailable.
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
// RequireOrg reads a path value, so register it on the mux that matches the pattern.
// Add v.Ready as a readiness check ("jwks") so a service is not ready without keys.
package auth
