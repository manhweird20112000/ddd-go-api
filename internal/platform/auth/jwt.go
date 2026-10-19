// Package auth signs and verifies Ed25519 access tokens without identity imports.
package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a bearer token cannot be trusted.
var ErrInvalidToken = errors.New("invalid access token")

// Issuer signs short-lived access tokens.
type Issuer struct {
	private  ed25519.PrivateKey
	issuer   string
	audience string
}

// NewIssuer decodes an Ed25519 private key from an environment value.
func NewIssuer(encoded, issuer, audience string) (*Issuer, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PrivateKeySize || issuer == "" || audience == "" {
		return nil, errors.New("invalid JWT signing configuration")
	}
	return &Issuer{private: ed25519.PrivateKey(key), issuer: issuer, audience: audience}, nil
}

// PublicKey returns a copy of the verifier key.
func (i *Issuer) PublicKey() ed25519.PublicKey {
	return append(ed25519.PublicKey(nil), i.private.Public().(ed25519.PublicKey)...)
}

// Issue signs a 15-minute EdDSA access token for subject.
func (i *Issuer) Issue(ctx context.Context, subject string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if subject == "" {
		return "", 0, errors.New("access token subject must not be empty")
	}
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Issuer: i.issuer, Audience: jwt.ClaimStrings{i.audience}, Subject: subject,
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.private)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return token, 900, nil
}

// Verifier checks EdDSA signatures and registered token claims.
type Verifier struct {
	public   ed25519.PublicKey
	issuer   string
	audience string
}

// NewVerifier creates a local verifier from the identity public key.
func NewVerifier(public ed25519.PublicKey, issuer, audience string) (*Verifier, error) {
	if len(public) != ed25519.PublicKeySize || issuer == "" || audience == "" {
		return nil, errors.New("invalid JWT verifier configuration")
	}
	return &Verifier{public: append(ed25519.PublicKey(nil), public...), issuer: issuer, audience: audience}, nil
}

// Verify returns the subject only for a valid access token.
func (v *Verifier) Verify(ctx context.Context, raw string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return v.public, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
