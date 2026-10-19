package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestIssueAndVerifyEdDSA(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewIssuer(base64.StdEncoding.EncodeToString(private), "go-template", "go-template-api")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(issuer.PublicKey(), "go-template", "go-template-api")
	if err != nil {
		t.Fatal(err)
	}
	token, expiresIn, err := issuer.Issue(context.Background(), "user-123")
	if err != nil || expiresIn != 900 {
		t.Fatalf("issue: expires=%d error=%v", expiresIn, err)
	}
	subject, err := verifier.Verify(context.Background(), token)
	if err != nil || subject != "user-123" {
		t.Fatalf("verify: subject=%q error=%v", subject, err)
	}
	if _, err := verifier.Verify(context.Background(), token+"x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}
