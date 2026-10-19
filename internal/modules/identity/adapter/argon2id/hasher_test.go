package argon2id

import (
	"context"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hasher := New()
	hash, err := hasher.Hash(context.Background(), "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct-password" {
		t.Fatal("password stored in clear text")
	}
	valid, err := hasher.Verify(context.Background(), hash, "correct-password")
	if err != nil || !valid {
		t.Fatalf("valid password rejected: %v, %v", valid, err)
	}
	valid, err = hasher.Verify(context.Background(), hash, "wrong-password")
	if err != nil || valid {
		t.Fatalf("wrong password accepted: %v, %v", valid, err)
	}
}

func TestMalformedHashIsRejected(t *testing.T) {
	_, err := New().Verify(context.Background(), "$argon2id$v=19$m=999999999,t=2,p=1$bad$bad", "password")
	if err == nil {
		t.Fatal("malformed hash accepted")
	}
}
