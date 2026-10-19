package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// Claims is the subset of token claims rzv checks or shows.
type Claims map[string]any

// String returns a string claim, or "".
func (c Claims) String(name string) string {
	value, _ := c[name].(string)
	return value
}

// Time returns a NumericDate claim, or the zero time.
func (c Claims) Time(name string) time.Time {
	value, ok := c[name].(float64)
	if !ok {
		return time.Time{}
	}
	return time.Unix(int64(value), 0).UTC()
}

func (c Claims) hasAudience(audience string) bool {
	switch aud := c["aud"].(type) {
	case string:
		return aud == audience
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && s == audience {
				return true
			}
		}
	}
	return false
}

// DecodeUnverified reads a JWT's claims without checking its signature. Only
// use it for display of a token that was verified when it was stored.
func DecodeUnverified(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	claims := Claims{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type keySet struct {
	keys []jwk
}

func fetchKeySet(ctx context.Context, client *http.Client, uri string) (*keySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("jwks unavailable")
	}
	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	return &keySet{keys: body.Keys}, nil
}

func (s *keySet) find(kid string) (*rsa.PublicKey, error) {
	var candidates []jwk
	for _, key := range s.keys {
		if key.Kty != "RSA" || (key.Use != "" && key.Use != "sig") || (key.Alg != "" && key.Alg != "RS256") {
			continue
		}
		if kid == "" || key.Kid == kid {
			candidates = append(candidates, key)
		}
	}
	if len(candidates) != 1 {
		return nil, errors.New("no unique signing key")
	}
	n, err := base64.RawURLEncoding.DecodeString(candidates[0].N)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(candidates[0].E)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil, errors.New("bad exponent")
	}
	exponent := 0
	for _, b := range e {
		exponent = exponent<<8 | int(b)
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
	if key.N.BitLen() < 2048 {
		return nil, errors.New("key too small")
	}
	return key, nil
}

// verifyRS256 checks the signature and returns the claims. Claim checks are
// the caller's job.
func (s *keySet) verifyRS256(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, err
	}
	if header.Alg != "RS256" {
		return nil, errors.New("unexpected algorithm")
	}
	key, err := s.find(header.Kid)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, err
	}
	return DecodeUnverified(token)
}

type tokenCheck struct {
	issuer       string
	audience     string
	clientID     string
	nonce        string // checked when not empty
	brokerClaim  string // checked equal to "github" when not empty
	requireTyp   bool   // access token must carry typ "Bearer"
	now          time.Time
	allowedSkew  time.Duration
	kindForError string
}

func (c tokenCheck) verify(set *keySet, token string) (Claims, error) {
	invalid := authError("Identity provider returned an invalid " + c.kindForError + " token.")
	claims, err := set.verifyRS256(token)
	if err != nil {
		return nil, invalid
	}
	exp, iat := claims.Time("exp"), claims.Time("iat")
	if exp.IsZero() || iat.IsZero() || claims.String("sub") == "" {
		return nil, invalid
	}
	if !c.now.Before(exp.Add(c.allowedSkew)) {
		return nil, authError("Identity provider returned an expired token. Sign in again.")
	}
	if claims.String("iss") != c.issuer || !claims.hasAudience(c.audience) || claims.String("azp") != c.clientID {
		return nil, invalid
	}
	if c.nonce != "" && claims.String("nonce") != c.nonce {
		return nil, invalid
	}
	if c.brokerClaim != "" && claims.String(c.brokerClaim) != "github" {
		return nil, invalid
	}
	if c.requireTyp && claims.String("typ") != "Bearer" {
		return nil, invalid
	}
	return claims, nil
}
