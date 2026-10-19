// Package argon2id implements the identity password-hashing port.
package argon2id

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memoryKiB   = 19 * 1024
	iterations  = 2
	parallelism = 1
	saltLength  = 16
	keyLength   = 32
)

// Hasher applies Argon2id with a random salt per password.
type Hasher struct{}

// New constructs the password hasher.
func New() *Hasher { return &Hasher{} }

// Hash encodes Argon2id parameters, salt, and derived key in PHC form.
func (*Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var salt [saltLength]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt[:], iterations, memoryKiB, parallelism, keyLength)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt[:]), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify checks a password in constant time after bounding stored parameters.
func (*Hasher) Verify(ctx context.Context, encoded, password string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("invalid password hash format")
	}
	var memory, passes, lanes uint32
	count, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &lanes)
	if err != nil || count != 3 || memory < memoryKiB || memory > 128*1024 || passes < 2 || passes > 10 || lanes < 1 || lanes > 8 {
		return false, errors.New("invalid password hash parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false, errors.New("invalid password hash salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != keyLength {
		return false, errors.New("invalid password hash key")
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, uint8(lanes), keyLength)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
