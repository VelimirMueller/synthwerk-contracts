package kit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/auth"
	"github.com/VelimirMueller/synthwerk-contracts/go/kit/auth/authtest"
)

func TestIDTokenVerifier(t *testing.T) {
	iss := authtest.NewIssuer(t)
	v, err := auth.NewIDTokenVerifier(iss.IDTokenConfig())
	if err != nil {
		t.Fatal(err)
	}
	signIn := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	idClaims := func(over map[string]any) map[string]any {
		c := authtest.IDTokenClaims("user-7", "sess-7", signIn, "pwd", "otp", "mfa")
		for k, val := range over {
			c[k] = val
		}
		return c
	}

	t.Run("Verify - Zitadel ID token - sub, auth_time, amr and sid", func(t *testing.T) {
		tok, err := v.Verify(t.Context(), iss.Mint(t, idClaims(nil)), "user-7")
		if err != nil {
			t.Fatal(err)
		}
		if tok.Subject != "user-7" || tok.SessionID != "sess-7" || !tok.AuthTime.Equal(signIn) || !tok.HasMFA() || tok.Raw["iss"] != iss.URL() {
			t.Fatalf("got %+v", tok)
		}
	})

	tests := []struct {
		name   string
		token  func(t *testing.T) string
		reason error
	}{
		{"Verify - access token (no auth_time) - rejects", func(t *testing.T) string {
			return iss.Mint(t, map[string]any{"sub": "user-7", "aud": []string{authtest.ClientID, "project-1"}})
		}, auth.ErrClaims},
		{"Verify - typ at+jwt - rejects", func(t *testing.T) string {
			return iss.MintType(t, "at+jwt", idClaims(nil))
		}, auth.ErrIDTokenType},
		{"Verify - other client in aud - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"aud": []string{"other-client"}, "azp": "other-client"}))
		}, auth.ErrAudience},
		{"Verify - azp of another client - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"aud": []string{authtest.ClientID, "other-client"}, "azp": "other-client"}))
		}, auth.ErrAudience},
		{"Verify - expired - rejects", func(t *testing.T) string {
			past := time.Now().Add(-time.Hour)
			return iss.Mint(t, idClaims(map[string]any{"iat": past.Unix(), "nbf": past.Unix(), "exp": past.Add(5 * time.Minute).Unix()}))
		}, auth.ErrExpired},
		{"Verify - wrong issuer - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"iss": "https://evil.example"}))
		}, auth.ErrIssuer},
		{"Verify - auth_time in the future - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"auth_time": time.Now().Add(10 * time.Minute).Unix()}))
		}, auth.ErrClaims},
		{"Verify - auth_time not a number - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"auth_time": "yesterday"}))
		}, auth.ErrClaims},
		{"Verify - no sub - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"sub": nil}))
		}, auth.ErrClaims},
		{"Verify - ID token of another user - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"sub": "user-8"}))
		}, auth.ErrSubject},
		{"Verify - azp not a string - rejects", func(t *testing.T) string {
			return iss.Mint(t, idClaims(map[string]any{"azp": 7}))
		}, auth.ErrAudience},
		{"Verify - key not in the JWKS - rejects", func(t *testing.T) string {
			return iss.MintUnpublished(t, idClaims(nil))
		}, auth.ErrUnknownKey},
		{"Verify - alg none - rejects", func(t *testing.T) string {
			return iss.MintNone(t, idClaims(nil))
		}, auth.ErrAlgorithm},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.Verify(t.Context(), tc.token(t), "user-7")
			if !errors.Is(err, auth.ErrUnauthenticated) || !errors.Is(err, tc.reason) {
				t.Fatalf("got %v, want ErrUnauthenticated and %v", err, tc.reason)
			}
		})
	}

	t.Run("Verify - empty expected subject - rejects", func(t *testing.T) {
		if _, err := v.Verify(t.Context(), iss.Mint(t, idClaims(nil)), ""); !errors.Is(err, auth.ErrSubject) {
			t.Fatalf("got %v, want ErrSubject", err)
		}
	})

	t.Run("Verify - no typ - accepts", func(t *testing.T) {
		if _, err := v.Verify(t.Context(), iss.MintType(t, "", idClaims(nil)), "user-7"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Verifier - ID token as access token - still refused", func(t *testing.T) {
		av := newVerifier(t, iss.Config())
		_, err := av.Verify(t.Context(), iss.Mint(t, idClaims(map[string]any{"aud": []string{authtest.Audience}})))
		if !errors.Is(err, auth.ErrTokenType) {
			t.Fatalf("got %v, want ErrTokenType", err)
		}
	})

	t.Run("NewIDTokenVerifier - at+jwt allowed - error", func(t *testing.T) {
		cfg := iss.IDTokenConfig()
		cfg.AcceptedTypes = []string{"at+jwt"}
		if _, err := auth.NewIDTokenVerifier(cfg); err == nil {
			t.Fatal("want an error")
		}
	})
}
