package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Defaults of Config.
const (
	DefaultLeeway          = 30 * time.Second
	DefaultMaxAge          = 15 * time.Minute
	DefaultRefreshInterval = 10 * time.Second
	DefaultFetchTimeout    = 5 * time.Second
)

// MaxLeeway is the largest clock skew Config accepts (spec #29: skew <= 60 s).
const MaxLeeway = 60 * time.Second

const maxTokenLen = 16 << 10

// algorithms is the allow-list. Never "none", never HS*.
var algorithms = []jose.SignatureAlgorithm{jose.RS256, jose.ES256}

// DefaultAcceptedTypes are the header "typ" values of an access token. "at+jwt" is
// RFC 9068; Zitadel access tokens carry "JWT" (confirmed v4.20.0).
var DefaultAcceptedTypes = []string{"at+jwt", "application/at+jwt", "JWT"}

// idTokenClaims occur only in OIDC ID tokens. A token with one is never an access token.
var idTokenClaims = []string{"nonce", "at_hash"}

// ErrUnauthenticated wraps every refusal of a token: the caller answers 401.
// errors.Is also matches the reason, for example ErrExpired.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

// ErrUnavailable means the key set cannot be loaded: the caller answers 503, not 401.
var ErrUnavailable = errors.New("auth: key set unavailable")

// Reasons, wrapped together with ErrUnauthenticated.
var (
	ErrMalformed   = errors.New("malformed token")
	ErrAlgorithm   = errors.New("algorithm not allowed")
	ErrTokenType   = errors.New("not an access token")
	ErrUnknownKey  = errors.New("unknown key id")
	ErrSignature   = errors.New("bad signature")
	ErrExpired     = errors.New("token expired")
	ErrNotYetValid = errors.New("token not yet valid")
	ErrIssuer      = errors.New("wrong issuer")
	ErrAudience    = errors.New("wrong audience")
	ErrClaims      = errors.New("missing or bad claims")
)

// Config sets up a Verifier. Issuer and Audiences are required.
type Config struct {
	// Issuer must equal the "iss" claim, for example "https://auth.synthwerk.dev".
	Issuer string
	// Audiences: the "aud" claim must contain at least one of them.
	Audiences []string
	// JWKSURL is the key set URL. Empty: read jwks_uri from
	// <Issuer>/.well-known/openid-configuration.
	JWKSURL string
	// Leeway is the clock skew allowed on exp, nbf and iat. Default 30 s, max 60 s.
	Leeway time.Duration
	// MaxAge is how long a fetched key set is used before a refresh. Default 15 min.
	MaxAge time.Duration
	// RefreshInterval is the min time between two fetches. It limits the refresh that
	// an unknown kid triggers. Default 10 s.
	RefreshInterval time.Duration
	// HTTPClient fetches the key set. Default: a client with a 5 s timeout.
	HTTPClient *http.Client
	// Mapper turns verified claims into a Principal. Default ZitadelMapper{}.
	Mapper ClaimsMapper
	// AcceptedTypes is the allow-list of the header "typ". Compared without case and
	// without the "application/" prefix. Nil: DefaultAcceptedTypes. A token without
	// typ passes only when the list contains "".
	AcceptedTypes []string
}

// Verifier checks access tokens. It is safe for concurrent use.
type Verifier struct {
	issuer    string
	audiences []string
	leeway    time.Duration
	mapper    ClaimsMapper
	types     map[string]bool
	keys      *keySet
}

// NewVerifier checks cfg and returns a Verifier. It does not fetch keys: the first
// Verify or Ready call does.
func NewVerifier(cfg Config) (*Verifier, error) {
	var errs []error
	if u, err := url.Parse(cfg.Issuer); cfg.Issuer == "" || err != nil || u.Host == "" {
		errs = append(errs, fmt.Errorf("issuer %q is not an absolute URL", cfg.Issuer))
	}
	if cfg.JWKSURL != "" {
		if u, err := url.Parse(cfg.JWKSURL); err != nil || u.Host == "" {
			errs = append(errs, fmt.Errorf("JWKS URL %q is not an absolute URL", cfg.JWKSURL))
		}
	}
	if len(cfg.Audiences) == 0 {
		errs = append(errs, errors.New("at least one audience is required"))
	}
	for _, a := range cfg.Audiences {
		if a == "" {
			errs = append(errs, errors.New("audience must not be empty"))
		}
	}
	if cfg.Leeway < 0 || cfg.Leeway > MaxLeeway {
		errs = append(errs, fmt.Errorf("leeway %s is not between 0 and %s", cfg.Leeway, MaxLeeway))
	}
	if cfg.MaxAge < 0 || cfg.RefreshInterval < 0 {
		errs = append(errs, errors.New("max age and refresh interval must not be negative"))
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("auth: %w", errors.Join(errs...))
	}

	if cfg.Leeway == 0 {
		cfg.Leeway = DefaultLeeway
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = DefaultMaxAge
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultRefreshInterval
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: DefaultFetchTimeout}
	}
	if cfg.Mapper == nil {
		cfg.Mapper = ZitadelMapper{}
	}
	if cfg.AcceptedTypes == nil {
		cfg.AcceptedTypes = DefaultAcceptedTypes
	}
	types := make(map[string]bool, len(cfg.AcceptedTypes))
	for _, t := range cfg.AcceptedTypes {
		types[normType(t)] = true
	}
	return &Verifier{
		issuer:    cfg.Issuer,
		audiences: append([]string(nil), cfg.Audiences...),
		leeway:    cfg.Leeway,
		mapper:    cfg.Mapper,
		types:     types,
		keys: &keySet{
			client:     cfg.HTTPClient,
			issuer:     cfg.Issuer,
			url:        cfg.JWKSURL,
			maxAge:     cfg.MaxAge,
			minRefresh: cfg.RefreshInterval,
		},
	}, nil
}

// Verify checks raw (the token without "Bearer ") and returns its Principal.
// A refused token gives an error that wraps ErrUnauthenticated and the reason.
// A key set that cannot be loaded gives ErrUnavailable.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Principal, error) {
	if raw == "" || len(raw) > maxTokenLen {
		return nil, refuse(ErrMalformed)
	}
	alg, kid, typ, err := peekHeader(raw)
	if err != nil {
		return nil, err
	}
	if !v.types[normType(typ)] {
		return nil, refuse(fmt.Errorf("%w: typ %q", ErrTokenType, typ))
	}
	tok, err := jwt.ParseSigned(raw, algorithms)
	if err != nil {
		return nil, refuse(ErrMalformed)
	}
	key, err := v.keys.get(ctx, kid)
	if err != nil {
		return nil, err
	}
	if !keyFits(key, alg) {
		return nil, refuse(ErrAlgorithm)
	}

	var std jwt.Claims
	var all map[string]any
	if err := tok.Claims(key.Key, &std, &all); err != nil {
		if errors.Is(err, jose.ErrCryptoFailure) {
			return nil, refuse(ErrSignature)
		}
		return nil, refuse(ErrMalformed)
	}
	for _, c := range idTokenClaims {
		if _, ok := all[c]; ok {
			return nil, refuse(fmt.Errorf("%w: ID token claim %s", ErrTokenType, c))
		}
	}
	if std.Expiry == nil {
		return nil, refuse(fmt.Errorf("%w: no exp", ErrClaims))
	}
	err = std.ValidateWithLeeway(jwt.Expected{Issuer: v.issuer, AnyAudience: v.audiences, Time: time.Now()}, v.leeway)
	switch {
	case err == nil:
	case errors.Is(err, jwt.ErrExpired):
		return nil, refuse(ErrExpired)
	case errors.Is(err, jwt.ErrNotValidYet), errors.Is(err, jwt.ErrIssuedInTheFuture):
		return nil, refuse(ErrNotYetValid)
	case errors.Is(err, jwt.ErrInvalidIssuer):
		return nil, refuse(ErrIssuer)
	case errors.Is(err, jwt.ErrInvalidAudience):
		return nil, refuse(ErrAudience)
	default:
		return nil, refuse(ErrClaims)
	}

	p, err := v.mapper.Map(all)
	if err != nil {
		return nil, refuse(fmt.Errorf("%w: %w", ErrClaims, err))
	}
	p.AMR = strs(all["amr"])
	p.Raw = all
	return p, nil
}

// Ready loads the key set when it is empty or older than MaxAge. Use it as the
// readiness check "jwks": it fails while no key is known.
func (v *Verifier) Ready(ctx context.Context) error { return v.keys.ready(ctx) }

func refuse(reason error) error { return fmt.Errorf("%w: %w", ErrUnauthenticated, reason) }

// normType makes "application/AT+JWT" and "at+jwt" equal (RFC 7515 §4.1.9).
func normType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	return strings.TrimPrefix(t, "application/")
}

// peekHeader reads alg, kid and typ before the library parses the token, so a token with
// a forbidden algorithm fails as ErrAlgorithm and never reaches a key lookup.
func peekHeader(raw string) (alg, kid, typ string, err error) {
	head, _, ok := strings.Cut(raw, ".")
	if !ok {
		return "", "", "", refuse(ErrMalformed)
	}
	b, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		return "", "", "", refuse(ErrMalformed)
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(b, &h); err != nil {
		return "", "", "", refuse(ErrMalformed)
	}
	if h.Alg != string(jose.RS256) && h.Alg != string(jose.ES256) {
		return "", "", "", refuse(ErrAlgorithm)
	}
	if h.Kid == "" {
		return "", "", "", refuse(fmt.Errorf("%w: no kid", ErrMalformed))
	}
	return h.Alg, h.Kid, h.Typ, nil
}

// keyFits reports whether the JWK may verify a token signed with alg.
func keyFits(k jose.JSONWebKey, alg string) bool {
	if k.Algorithm != "" && k.Algorithm != alg {
		return false
	}
	switch pub := k.Key.(type) {
	case *rsa.PublicKey:
		return alg == string(jose.RS256)
	case *ecdsa.PublicKey:
		return alg == string(jose.ES256) && pub.Curve == elliptic.P256()
	default:
		return false
	}
}
