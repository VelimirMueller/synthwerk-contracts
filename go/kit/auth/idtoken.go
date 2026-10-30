package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// DefaultIDTokenTypes are the header "typ" values of an ID token. Zitadel ID tokens
// carry "JWT" (v4.20.0). OIDC Core requires no typ, so a token without one passes.
// "at+jwt" never passes: it marks an access token (RFC 9068).
var DefaultIDTokenTypes = []string{"JWT", ""}

// IDTokenVerifier checks OIDC ID tokens of the identity provider (synthwerk#41). A
// service uses one to read proof of a recent sign-in (auth_time, amr, sid) that the
// access token does not carry. It is safe for concurrent use.
type IDTokenVerifier struct {
	v       *Verifier
	clients []string
}

// IDToken is a verified ID token.
type IDToken struct {
	// Subject is the "sub" claim.
	Subject string
	// AuthTime is the "auth_time" claim: when the user last authenticated.
	AuthTime time.Time
	// AMR are the authentication methods of that sign-in ("amr").
	AMR []string
	// SessionID is the "sid" claim of the identity provider session. Empty when absent.
	SessionID string
	// Raw holds every verified claim.
	Raw map[string]any
}

// HasMFA reports whether the sign-in used more than one factor (amr contains "mfa").
func (t *IDToken) HasMFA() bool { return slices.Contains(t.AMR, "mfa") }

// NewIDTokenVerifier checks cfg and returns an IDTokenVerifier. cfg.Audiences are the
// OIDC client ids whose ID tokens pass (not the project id: every access token of
// the project has that). Mapper is not used. AcceptedTypes nil: DefaultIDTokenTypes.
func NewIDTokenVerifier(cfg Config) (*IDTokenVerifier, error) {
	if cfg.AcceptedTypes == nil {
		cfg.AcceptedTypes = DefaultIDTokenTypes
	}
	for _, t := range cfg.AcceptedTypes {
		if normType(t) == "at+jwt" {
			return nil, fmt.Errorf("auth: typ %q marks an access token", t)
		}
	}
	v, err := NewVerifier(cfg)
	if err != nil {
		return nil, err
	}
	return &IDTokenVerifier{v: v, clients: append([]string(nil), cfg.Audiences...)}, nil
}

// ErrSubject: the ID token belongs to another user than the caller.
var ErrSubject = errors.New("subject differs")

// Verify checks raw like Verifier.Verify does (signature, iss, aud, exp, nbf, iat),
// then the ID token rules:
//   - "sub" equals subject, the subject of the caller's access token. An empty
//     subject refuses every token.
//   - "auth_time" is set and not in the future (leeway). A token without it is no
//     proof of a sign-in: Zitadel access tokens do not have it.
//   - "azp", when set, is one of the client ids.
//
// The nonce is not checked: only the client that started the sign-in knows it.
// A refused token gives an error that wraps ErrUnauthenticated and the reason.
func (iv *IDTokenVerifier) Verify(ctx context.Context, raw, subject string) (*IDToken, error) {
	all, err := iv.v.verify(ctx, raw, false)
	if err != nil {
		return nil, err
	}
	sub, _ := all["sub"].(string)
	if sub == "" {
		return nil, refuse(fmt.Errorf("%w: no sub", ErrClaims))
	}
	if subject == "" || sub != subject {
		return nil, refuse(ErrSubject)
	}
	if azp, ok := all["azp"]; ok {
		if s, isStr := azp.(string); !isStr || !slices.Contains(iv.clients, s) {
			return nil, refuse(fmt.Errorf("%w: azp is not an accepted client", ErrAudience))
		}
	}
	at, ok := numericDate(all["auth_time"])
	if !ok {
		return nil, refuse(fmt.Errorf("%w: no auth_time", ErrClaims))
	}
	if at.After(time.Now().Add(iv.v.leeway)) {
		return nil, refuse(fmt.Errorf("%w: auth_time in the future", ErrClaims))
	}
	sid, _ := all["sid"].(string)
	return &IDToken{Subject: sub, AuthTime: at, AMR: strs(all["amr"]), SessionID: sid, Raw: all}, nil
}

// Ready loads the key set, like Verifier.Ready.
func (iv *IDTokenVerifier) Ready(ctx context.Context) error { return iv.v.Ready(ctx) }

// numericDate reads a JWT NumericDate (seconds since the epoch, RFC 7519).
func numericDate(v any) (time.Time, bool) {
	var sec float64
	switch n := v.(type) {
	case float64:
		sec = n
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return time.Time{}, false
		}
		sec = f
	default:
		return time.Time{}, false
	}
	if sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(sec), 0), true
}
