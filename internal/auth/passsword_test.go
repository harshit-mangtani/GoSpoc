package auth

import "testing"

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	password := "test-password"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty encoded hash")
	}

	ok, err := VerifyPassword(password, hash)

	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}

	if !ok {
		t.Fatal("expected password verification to succeed")
	}

}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	password := "super-secret-password"
	wrongPassword := "wrong-password"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	ok, err := VerifyPassword(wrongPassword, hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}

	if ok {
		t.Fatal("expected password verification to fail for wrong password")
	}
}


func TestVerifyPasswordRejectsInvalidHash(t *testing.T) {
	ok, err := VerifyPassword("anything", "not-a-valid-hash")
	if err == nil {
		t.Fatal("expected error for invalid encoded hash")
	}

	if ok {
		t.Fatal("expected verification to fail for invalid encoded hash")
	}
}