// Command devenv creates a local .env with fresh random credentials.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("created .env with local development credentials")
}

func generate() error {
	example, err := os.ReadFile(".env.example")
	if err != nil {
		return fmt.Errorf("read .env.example: %w", err)
	}
	postgres, err := randomHex(24)
	if err != nil {
		return err
	}
	identity, err := randomHex(24)
	if err != nil {
		return err
	}
	notification, err := randomHex(24)
	if err != nil {
		return err
	}
	key, err := randomBytes(32)
	if err != nil {
		return err
	}
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate Ed25519 signing key: %w", err)
	}
	values := map[string]string{
		"POSTGRES_PASSWORD":         postgres,
		"IDENTITY_DB_PASSWORD":      identity,
		"NOTIFICATION_DB_PASSWORD":  notification,
		"DATABASE_URL":              "postgres://template:" + postgres + "@localhost:5432/template?sslmode=disable",
		"IDENTITY_DATABASE_URL":     "postgres://identity_app:" + identity + "@localhost:5432/template?sslmode=disable",
		"NOTIFICATION_DATABASE_URL": "postgres://notification_app:" + notification + "@localhost:5432/template?sslmode=disable",
		"JWT_PRIVATE_KEY_BASE64":    base64.StdEncoding.EncodeToString(signing),
		"IDEMPOTENCY_KEY_BASE64":    base64.StdEncoding.EncodeToString(key),
	}
	var output strings.Builder
	for _, line := range strings.Split(string(example), "\n") {
		name, _, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "=")
		if value, exists := values[name]; ok && exists {
			line = name + "=" + value
		}
		output.WriteString(strings.TrimSuffix(line, "\r"))
		output.WriteByte('\n')
	}
	file, err := os.OpenFile(".env", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create .env without replacing an existing file: %w", err)
	}
	if _, err := file.WriteString(output.String()); err != nil {
		_ = file.Close()
		return fmt.Errorf("write .env: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close .env: %w", err)
	}
	return nil
}

func randomHex(size int) (string, error) {
	value, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate random credential: %w", err)
	}
	return value, nil
}
