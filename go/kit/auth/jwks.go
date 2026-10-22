package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

const (
	maxBodyLen    = 1 << 20
	minRSABits    = 2048
	discoveryPath = "/.well-known/openid-configuration"
)

// keySet caches the issuer JWKS. Reads take a read lock only. One fetch runs at a
// time; callers that waited for it use its result instead of fetching again.
type keySet struct {
	client     *http.Client
	issuer     string
	url        string // empty until discovery, guarded by refreshMu
	maxAge     time.Duration
	minRefresh time.Duration

	mu      sync.RWMutex
	keys    map[string]jose.JSONWebKey
	fetched time.Time

	refreshMu   sync.Mutex
	lastAttempt time.Time
}

func (s *keySet) lookup(kid string) (key jose.JSONWebKey, found, fresh, empty bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, found = s.keys[kid]
	return key, found, !s.fetched.IsZero() && time.Since(s.fetched) < s.maxAge, len(s.keys) == 0
}

// get returns the key for kid. An unknown kid or an old key set triggers one
// refresh, at most once per minRefresh. When the refresh fails, a known key stays usable.
func (s *keySet) get(ctx context.Context, kid string) (jose.JSONWebKey, error) {
	key, found, fresh, _ := s.lookup(kid)
	if found && fresh {
		return key, nil
	}
	if err := s.refresh(ctx); err != nil && !found {
		return jose.JSONWebKey{}, err
	}
	key, found, _, empty := s.lookup(kid)
	switch {
	case found:
		return key, nil
	case empty:
		return jose.JSONWebKey{}, fmt.Errorf("%w: no key loaded", ErrUnavailable)
	default:
		return jose.JSONWebKey{}, refuse(ErrUnknownKey)
	}
}

func (s *keySet) ready(ctx context.Context) error {
	_, _, fresh, empty := s.lookup("")
	if fresh && !empty {
		return nil
	}
	err := s.refresh(ctx)
	if _, _, _, empty := s.lookup(""); empty {
		if err == nil {
			err = fmt.Errorf("%w: no key loaded", ErrUnavailable)
		}
		return err
	}
	return nil
}

// refresh fetches the key set unless a fetch ran less than minRefresh ago.
func (s *keySet) refresh(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if !s.lastAttempt.IsZero() && time.Since(s.lastAttempt) < s.minRefresh {
		return nil
	}
	s.lastAttempt = time.Now()

	// A caller that gives up must not cancel a fetch that all callers share.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultFetchTimeout)
	defer cancel()
	keys, err := s.fetch(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	s.mu.Lock()
	s.keys, s.fetched = keys, time.Now()
	s.mu.Unlock()
	return nil
}

func (s *keySet) fetch(ctx context.Context) (map[string]jose.JSONWebKey, error) {
	if s.url == "" {
		var doc struct {
			Issuer  string `json:"issuer"`
			JWKSURI string `json:"jwks_uri"`
		}
		if err := getJSON(ctx, s.client, strings.TrimSuffix(s.issuer, "/")+discoveryPath, &doc); err != nil {
			return nil, fmt.Errorf("discovery: %w", err)
		}
		if doc.Issuer != s.issuer || doc.JWKSURI == "" {
			return nil, errors.New("discovery: issuer differs or jwks_uri is missing")
		}
		s.url = doc.JWKSURI
	}

	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := getJSON(ctx, s.client, s.url, &set); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	keys := make(map[string]jose.JSONWebKey, len(set.Keys))
	for _, raw := range set.Keys {
		// Skip keys this verifier cannot use (other types, encryption keys) one by one.
		var k jose.JSONWebKey
		if err := k.UnmarshalJSON(raw); err != nil || !usable(k) {
			continue
		}
		keys[k.KeyID] = k
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks: no usable RS256 or ES256 signing key")
	}
	return keys, nil
}

func usable(k jose.JSONWebKey) bool {
	if k.KeyID == "" || !k.Valid() || !k.IsPublic() || (k.Use != "" && k.Use != "sig") {
		return false
	}
	switch pub := k.Key.(type) {
	case *rsa.PublicKey:
		return pub.N.BitLen() >= minRSABits && (k.Algorithm == "" || k.Algorithm == string(jose.RS256))
	case *ecdsa.PublicKey:
		return pub.Curve == elliptic.P256() && (k.Algorithm == "" || k.Algorithm == string(jose.ES256))
	default:
		return false
	}
}

func getJSON(ctx context.Context, client *http.Client, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodyLen)).Decode(out)
}
