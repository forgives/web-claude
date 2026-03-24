package auth

import "testing"

func TestHashPasswordAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "secret123" {
		t.Fatal("password should be hashed")
	}
	if !CheckPassword(hash, "secret123") {
		t.Fatal("expected password to match hash")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail")
	}
}
