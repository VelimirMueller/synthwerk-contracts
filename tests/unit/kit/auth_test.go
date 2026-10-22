package kit_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/auth"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/auth/authtest"
)

func newVerifier(t *testing.T, cfg auth.Config) *auth.Verifier {
	t.Helper()
	v, err := auth.NewVerifier(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVerify(t *testing.T) {
	iss := authtest.NewIssuer(t)
	v := newVerifier(t, iss.Config())
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name   string
		token  func(t *testing.T) string
		reason error
	}{
		{"Verify - valid token - principal", func(t *testing.T) string {
			return iss.Mint(t, authtest.Claims("user-7", "org-7", auth.RoleAdmin))
		}, nil},
		{"Verify - expired token - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"iat": past.Unix(), "nbf": past.Unix(), "exp": past.Add(5 * time.Minute).Unix()})
		}, auth.ErrExpired},
		{"Verify - expired within the leeway - accepts", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"exp": time.Now().Add(-10 * time.Second).Unix()})
		}, nil},
		{"Verify - nbf in the future - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"nbf": time.Now().Add(10 * time.Minute).Unix()})
		}, auth.ErrNotYetValid},
		{"Verify - no exp - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"exp": nil})
		}, auth.ErrClaims},
		{"Verify - wrong aud - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"aud": []string{"other-api"}})
		}, auth.ErrAudience},
		{"Verify - wrong iss - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"iss": "https://evil.example"})
		}, auth.ErrIssuer},
		{"Verify - alg none - rejects", func(t *testing.T) string {
			return iss.MintNone(t, nil)
		}, auth.ErrAlgorithm},
		{"Verify - alg HS256 - rejects", func(t *testing.T) string {
			return iss.MintHS256(t, nil)
		}, auth.ErrAlgorithm},
		{"Verify - signature changed - rejects", func(t *testing.T) string {
			// Claims of one token, signature of another token of the same key.
			a := strings.Split(iss.Mint(t, map[string]any{"sub": "user-a"}), ".")
			b := strings.Split(iss.Mint(t, map[string]any{"sub": "user-b"}), ".")
			return a[0] + "." + a[1] + "." + b[2]
		}, auth.ErrSignature},
		{"Verify - not a JWT - rejects", func(*testing.T) string { return "abc.def" }, auth.ErrMalformed},
		{"Claims - missing sub - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"sub": nil})
		}, auth.ErrClaims},
		{"Claims - missing org - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{auth.ClaimOrgID: nil})
		}, auth.ErrClaims},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := v.Verify(t.Context(), tc.token(t))
			if tc.reason == nil {
				if err != nil {
					t.Fatalf("got %v, want a principal", err)
				}
				if p.Subject == "" || p.OrgID == "" || p.Raw["iss"] != iss.URL() {
					t.Fatalf("principal %+v lacks sub, org or raw claims", p)
				}
				return
			}
			if !errors.Is(err, auth.ErrUnauthenticated) || !errors.Is(err, tc.reason) {
				t.Fatalf("got %v, want ErrUnauthenticated and %v", err, tc.reason)
			}
		})
	}

	t.Run("Verify - valid ES256 token - principal with org roles", func(t *testing.T) {
		iss := authtest.NewIssuer(t)
		iss.Rotate(t, jose.ES256)
		v := newVerifier(t, iss.Config())
		p, err := v.Verify(t.Context(), iss.Mint(t, authtest.Claims("user-7", "org-7", auth.RoleMember, auth.RoleAdmin)))
		if err != nil {
			t.Fatal(err)
		}
		if p.Subject != "user-7" || p.OrgID != "org-7" || len(p.Roles) != 2 || !p.HasRole(auth.RoleAdmin) {
			t.Fatalf("got %+v, want user-7 in org-7 with admin and member", p)
		}
	})
}

func TestVerifyKeys(t *testing.T) {
	t.Run("Verify - unknown kid - refreshes JWKS once, then rejects", func(t *testing.T) {
		iss := authtest.NewIssuer(t)
		v := newVerifier(t, iss.Config())
		if _, err := v.Verify(t.Context(), iss.Mint(t, nil)); err != nil {
			t.Fatal(err)
		}
		before := iss.Hits()
		_, err := v.Verify(t.Context(), iss.MintUnpublished(t, nil))
		if !errors.Is(err, auth.ErrUnknownKey) {
			t.Fatalf("got %v, want ErrUnknownKey", err)
		}
		// Within RefreshInterval a second unknown kid does not fetch again.
		_, err = v.Verify(t.Context(), iss.MintUnpublished(t, nil))
		if !errors.Is(err, auth.ErrUnknownKey) {
			t.Fatalf("got %v, want ErrUnknownKey", err)
		}
		if got := iss.Hits() - before; got > 1 {
			t.Fatalf("%d JWKS fetches for unknown kids, want at most 1", got)
		}
	})

	t.Run("Verify - key rotated after the first fetch - refreshes once, then accepts", func(t *testing.T) {
		iss := authtest.NewIssuer(t)
		cfg := iss.Config()
		cfg.RefreshInterval = time.Nanosecond
		v := newVerifier(t, cfg)
		if _, err := v.Verify(t.Context(), iss.Mint(t, nil)); err != nil {
			t.Fatal(err)
		}
		iss.Rotate(t, jose.ES256)
		before := iss.Hits()
		if _, err := v.Verify(t.Context(), iss.Mint(t, nil)); err != nil {
			t.Fatalf("token of the rotated key: %v", err)
		}
		if got := iss.Hits() - before; got != 1 {
			t.Fatalf("%d JWKS fetches, want 1", got)
		}
		if _, err := v.Verify(t.Context(), iss.Mint(t, nil)); err != nil || iss.Hits()-before != 1 {
			t.Fatalf("second token: %v after %d fetches, want cached key", err, iss.Hits()-before)
		}
	})

	t.Run("Verify - JWKS unreachable - ErrUnavailable", func(t *testing.T) {
		iss := authtest.NewIssuer(t)
		cfg := iss.Config()
		cfg.JWKSURL = iss.URL() + "/missing"
		v := newVerifier(t, cfg)
		_, err := v.Verify(t.Context(), iss.Mint(t, nil))
		if !errors.Is(err, auth.ErrUnavailable) || errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("got %v, want ErrUnavailable only", err)
		}
		if err := v.Ready(t.Context()); err == nil {
			t.Fatal("Ready: no error without keys")
		}
	})

	t.Run("Ready - JWKS reachable - nil", func(t *testing.T) {
		iss := authtest.NewIssuer(t)
		cfg := iss.Config()
		cfg.JWKSURL = iss.JWKSURL()
		if err := newVerifier(t, cfg).Ready(t.Context()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("NewVerifier - no issuer, no audience, leeway too large - error", func(t *testing.T) {
		if _, err := auth.NewVerifier(auth.Config{Leeway: 2 * time.Minute}); err == nil {
			t.Fatal("no error")
		}
	})
}

func TestZitadelMapper(t *testing.T) {
	claims := func(roles map[string]any) map[string]any {
		return map[string]any{"sub": "u", auth.ClaimOrgID: "org-1", auth.ClaimRoles: roles, auth.ClaimScope: "user tenant"}
	}
	t.Run("Map - role granted in another org - not kept", func(t *testing.T) {
		p, err := auth.ZitadelMapper{}.Map(claims(map[string]any{
			auth.RoleOwner: map[string]any{"org-2": "x"},
			auth.RoleAdmin: map[string]any{"org-1": "x"},
		}))
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Roles) != 1 || p.Roles[0] != auth.RoleAdmin {
			t.Fatalf("roles %v, want [admin]", p.Roles)
		}
		if !p.HasScope("user") || !p.HasScope("tenant") {
			t.Fatalf("scopes %v, want user and tenant", p.Scopes)
		}
	})

	t.Run("Map - operator granted in the platform org - operator", func(t *testing.T) {
		roles := map[string]any{auth.RoleOperator: map[string]any{"org-platform": "x"}}
		p, err := auth.ZitadelMapper{PlatformOrgID: "org-platform"}.Map(claims(roles))
		if err != nil || !p.IsOperator() {
			t.Fatalf("got %+v %v, want operator", p, err)
		}
		p, _ = auth.ZitadelMapper{}.Map(claims(roles))
		if p.IsOperator() {
			t.Fatal("operator without a platform org id")
		}
	})
}

type problemBody struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
}

func TestMiddleware(t *testing.T) {
	iss := authtest.NewIssuer(t)
	v := newVerifier(t, iss.Config())
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, found := auth.FromContext(r.Context()); !found {
			t.Error("no principal in the context")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	api := http.NewServeMux()
	api.Handle("GET /v1/orgs/{org}/members", auth.RequireOrg("org")(auth.RequireRole(auth.RoleAdmin, auth.RoleOwner)(ok)))
	api.Handle("GET /v1/chat", auth.RequireScope("user")(ok))
	handler := auth.Middleware(v, api)

	admin := iss.Mint(t, authtest.Claims("u1", "org-1", auth.RoleAdmin))
	member := iss.Mint(t, authtest.Claims("u2", "org-1", auth.RoleMember))

	tests := []struct {
		name   string
		path   string
		header string
		status int
		code   string
	}{
		{"Middleware - admin of the org - 204", "/v1/orgs/org-1/members", "Bearer " + admin, http.StatusNoContent, ""},
		{"Middleware - lowercase bearer scheme - 204", "/v1/orgs/org-1/members", "bearer " + admin, http.StatusNoContent, ""},
		{"Middleware - missing bearer - 401", "/v1/orgs/org-1/members", "", http.StatusUnauthorized, auth.CodeUnauthenticated},
		{"Middleware - basic scheme - 401", "/v1/orgs/org-1/members", "Basic dTpw", http.StatusUnauthorized, auth.CodeUnauthenticated},
		{"Middleware - expired token - 401", "/v1/orgs/org-1/members", "Bearer " + iss.Mint(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}), http.StatusUnauthorized, auth.CodeUnauthenticated},
		{"RequireRole - member on an admin route - 403", "/v1/orgs/org-1/members", "Bearer " + member, http.StatusForbidden, auth.CodeForbidden},
		{"RequireOrg - admin of another org - 403", "/v1/orgs/org-2/members", "Bearer " + admin, http.StatusForbidden, auth.CodeForbidden},
		{"RequireScope - no user scope - 403", "/v1/chat", "Bearer " + admin, http.StatusForbidden, auth.CodeForbidden},
		{"RequireScope - user scope - 204", "/v1/chat", "Bearer " + iss.Mint(t, map[string]any{auth.ClaimScope: "user"}), http.StatusNoContent, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.status, rec.Body)
			}
			if tc.code == "" {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != auth.ProblemContentType {
				t.Fatalf("content type %q, want %s", ct, auth.ProblemContentType)
			}
			var body problemBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tc.status || body.Code != tc.code || body.Title == "" {
				t.Fatalf("body %+v, want status %d code %s", body, tc.status, tc.code)
			}
			if tc.status == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("401 without WWW-Authenticate")
			}
		})
	}

	t.Run("RequireRole - no Middleware in front - 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		auth.RequireRole(auth.RoleAdmin)(ok).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	})
}
