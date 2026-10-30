package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TokenVerifier checks a raw access token. *Verifier and *MultiVerifier implement it.
type TokenVerifier interface {
	// Verify checks raw (the token without "Bearer ") and returns its Principal.
	Verify(ctx context.Context, raw string) (*Principal, error)
	// Ready fails while a key set cannot be loaded.
	Ready(ctx context.Context) error
}

var (
	_ TokenVerifier = (*Verifier)(nil)
	_ TokenVerifier = (*MultiVerifier)(nil)
)

// MultiVerifier accepts the tokens of more than one issuer, for example Zitadel user
// tokens and the widget tokens of synthwerk-identity (synthwerk#30).
//
//   - It reads the unverified "iss" claim only to pick the Verifier with that exact
//     issuer. That Verifier then checks the signature against its own JWKS, and iss,
//     aud, exp, nbf and iat. A token cannot pick the keys of another issuer.
//   - A token of an unknown issuer is refused (ErrIssuer) and no key set is fetched.
//   - Each issuer keeps its own audiences, mapper and key cache.
//   - Principal.Issuer tells the issuer. Use RequireIssuer to keep a route to one issuer.
type MultiVerifier struct {
	byIssuer map[string]*Verifier
	order    []*Verifier
}

// NewMultiVerifier returns a MultiVerifier over vs. It needs at least one Verifier,
// and the issuers must differ.
func NewMultiVerifier(vs ...*Verifier) (*MultiVerifier, error) {
	if len(vs) == 0 {
		return nil, errors.New("auth: at least one verifier is required")
	}
	m := &MultiVerifier{byIssuer: make(map[string]*Verifier, len(vs))}
	for _, v := range vs {
		if v == nil {
			return nil, errors.New("auth: verifier must not be nil")
		}
		if _, dup := m.byIssuer[v.issuer]; dup {
			return nil, fmt.Errorf("auth: issuer %q is set twice", v.issuer)
		}
		m.byIssuer[v.issuer] = v
		m.order = append(m.order, v)
	}
	return m, nil
}

// Verify picks the Verifier of the token's "iss" and lets it check the token.
// The errors are those of Verifier.Verify.
func (m *MultiVerifier) Verify(ctx context.Context, raw string) (*Principal, error) {
	if raw == "" || len(raw) > maxTokenLen {
		return nil, refuse(ErrMalformed)
	}
	iss, err := peekIssuer(raw)
	if err != nil {
		return nil, err
	}
	v, ok := m.byIssuer[iss]
	if !ok {
		return nil, refuse(ErrIssuer)
	}
	return v.Verify(ctx, raw)
}

// Ready checks the key set of every issuer. It fails when one fails.
func (m *MultiVerifier) Ready(ctx context.Context) error {
	var errs []error
	for _, v := range m.order {
		if err := v.Ready(ctx); err != nil {
			errs = append(errs, fmt.Errorf("issuer %s: %w", v.issuer, err))
		}
	}
	return errors.Join(errs...)
}

// peekIssuer reads "iss" from the payload of a compact JWS without a signature check.
func peekIssuer(raw string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", refuse(ErrMalformed)
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", refuse(ErrMalformed)
	}
	var c struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return "", refuse(ErrMalformed)
	}
	if c.Iss == "" {
		return "", refuse(ErrIssuer)
	}
	return c.Iss, nil
}
