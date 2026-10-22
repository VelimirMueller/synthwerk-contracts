// Package authtest is a fake token issuer for tests. It serves OIDC discovery and a
// JWKS over httptest and mints signed tokens with the claims a test gives, so a
// service tests package auth without Zitadel.
//
//	iss := authtest.NewIssuer(t)
//	v, _ := auth.NewVerifier(iss.Config())
//	tok := iss.Mint(t, authtest.Claims("user-1", "org-1", auth.RoleAdmin))
//	req.Header.Set("Authorization", "Bearer "+tok)
package authtest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/VelimirMueller/synthwerk-contracts/go/kit/auth"
)

// Defaults of a minted token.
const (
	Audience = "synthwerk-api"
	Subject  = "user-1"
	OrgID    = "org-1"
	Lifetime = 5 * time.Minute
)

// JWKSPath is where the issuer serves its key set (the Zitadel path).
const JWKSPath = "/oauth/v2/keys"

type signingKey struct {
	id   string
	alg  jose.SignatureAlgorithm
	priv crypto.Signer
}

// Issuer is a running fake issuer. Close happens on test cleanup.
type Issuer struct {
	srv  *httptest.Server
	hits atomic.Int64

	mu        sync.Mutex
	published []signingKey
	current   signingKey
}

// NewIssuer starts an issuer with one published RS256 key.
func NewIssuer(t testing.TB) *Issuer {
	t.Helper()
	i := &Issuer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"issuer": i.URL(), "jwks_uri": i.JWKSURL()})
	})
	mux.HandleFunc("GET "+JWKSPath, func(w http.ResponseWriter, _ *http.Request) {
		i.hits.Add(1)
		i.mu.Lock()
		set := jose.JSONWebKeySet{}
		for _, k := range i.published {
			set.Keys = append(set.Keys, jose.JSONWebKey{Key: k.priv.Public(), KeyID: k.id, Algorithm: string(k.alg), Use: "sig"})
		}
		i.mu.Unlock()
		writeJSON(w, set)
	})
	i.srv = httptest.NewServer(mux)
	t.Cleanup(i.srv.Close)
	i.Rotate(t, jose.RS256)
	return i
}

// URL is the issuer URL: the "iss" claim and Config.Issuer.
func (i *Issuer) URL() string { return i.srv.URL }

// JWKSURL is the URL of the key set.
func (i *Issuer) JWKSURL() string { return i.srv.URL + JWKSPath }

// Hits is the number of key set requests so far.
func (i *Issuer) Hits() int { return int(i.hits.Load()) }

// Config returns a verifier config for this issuer and Audience. It uses discovery,
// like a service with only an issuer URL.
func (i *Issuer) Config() auth.Config {
	return auth.Config{Issuer: i.URL(), Audiences: []string{Audience}, HTTPClient: i.srv.Client()}
}

// Rotate adds a new published key (RS256 or ES256) and signs the next tokens with it.
// Old keys stay published. It returns the new kid.
func (i *Issuer) Rotate(t testing.TB, alg jose.SignatureAlgorithm) string {
	t.Helper()
	k := newKey(t, alg)
	i.mu.Lock()
	defer i.mu.Unlock()
	i.published = append(i.published, k)
	i.current = k
	return k.id
}

// Mint returns a token signed with the current key. It sets iss, aud, sub, org id,
// iat, nbf and exp (Lifetime); claims override them, and a nil value removes a claim.
func (i *Issuer) Mint(t testing.TB, claims map[string]any) string {
	t.Helper()
	i.mu.Lock()
	k := i.current
	i.mu.Unlock()
	return sign(t, k.alg, jose.JSONWebKey{Key: k.priv, KeyID: k.id}, i.claims(claims))
}

// MintUnpublished returns a token signed with a new key that the JWKS does not have.
func (i *Issuer) MintUnpublished(t testing.TB, claims map[string]any) string {
	t.Helper()
	k := newKey(t, jose.RS256)
	return sign(t, k.alg, jose.JSONWebKey{Key: k.priv, KeyID: k.id}, i.claims(claims))
}

// MintHS256 returns a token signed with HS256 under the kid of the current key.
// A verifier must refuse it.
func (i *Issuer) MintHS256(t testing.TB, claims map[string]any) string {
	t.Helper()
	i.mu.Lock()
	kid := i.current.id
	i.mu.Unlock()
	return sign(t, jose.HS256, jose.JSONWebKey{Key: []byte("0123456789abcdef0123456789abcdef"), KeyID: kid}, i.claims(claims))
}

// MintNone returns an unsigned token (alg "none"). A verifier must refuse it.
func (i *Issuer) MintNone(t testing.TB, claims map[string]any) string {
	t.Helper()
	i.mu.Lock()
	kid := i.current.id
	i.mu.Unlock()
	head := mustJSON(t, map[string]string{"alg": "none", "typ": "JWT", "kid": kid})
	body := mustJSON(t, i.claims(claims))
	return base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body) + "."
}

// Claims returns the Zitadel claims for sub in org with the org roles. Pass the
// result to Mint.
func Claims(sub, org string, roles ...string) map[string]any {
	return map[string]any{"sub": sub, auth.ClaimOrgID: org, auth.ClaimRoles: Roles(org, roles...)}
}

// Roles returns a Zitadel roles claim that grants roles in org.
func Roles(org string, roles ...string) map[string]any {
	out := map[string]any{}
	for _, r := range roles {
		out[r] = map[string]any{org: org + ".synthwerk.test"}
	}
	return out
}

func (i *Issuer) claims(over map[string]any) map[string]any {
	now := time.Now()
	c := map[string]any{
		"iss":           i.URL(),
		"aud":           []string{Audience},
		"sub":           Subject,
		auth.ClaimOrgID: OrgID,
		"iat":           now.Unix(),
		"nbf":           now.Unix(),
		"exp":           now.Add(Lifetime).Unix(),
	}
	maps.Copy(c, over)
	maps.DeleteFunc(c, func(_ string, v any) bool { return v == nil })
	return c
}

func newKey(t testing.TB, alg jose.SignatureAlgorithm) signingKey {
	t.Helper()
	var priv crypto.Signer
	var err error
	switch alg { //nolint:exhaustive // the verifier allows only these two
	case jose.RS256:
		priv, err = rsa.GenerateKey(rand.Reader, 2048)
	case jose.ES256:
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	default:
		t.Fatalf("authtest: algorithm %s is not supported", alg)
	}
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	return signingKey{id: hex.EncodeToString(id), alg: alg, priv: priv}
}

func sign(t testing.TB, alg jose.SignatureAlgorithm, key jose.JSONWebKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func mustJSON(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
