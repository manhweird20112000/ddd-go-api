package domain

import "testing"

func TestNormalizeEmail(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
		valid bool
	}{
		{input: " Alice@Example.COM ", want: "alice@example.com", valid: true},
		{input: "not-an-email", valid: false},
		{input: "Alice <alice@example.com>", valid: false},
	} {
		got, err := NormalizeEmail(tc.input)
		if tc.valid && (err != nil || got != tc.want) {
			t.Fatalf("%q: got %q, %v", tc.input, got, err)
		}
		if !tc.valid && err == nil {
			t.Fatalf("%q: expected validation error", tc.input)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := ValidatePassword("length-twelve"); err != nil {
		t.Fatal(err)
	}
}
