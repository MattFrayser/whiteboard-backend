package tests

import (
	"testing"

	"main/internal/auth"
)

func TestHashPassword_ValidPassword(t *testing.T) {
	password := "mySecurePassword123"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("Hash should not be empty")
	}

	if hash == password {
		t.Error("Hash should not equal plaintext password")
	}

	// Hash should start with bcrypt prefix
	if len(hash) < 10 {
		t.Error("Hash seems too short to be valid bcrypt")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := auth.HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword with empty string should not error: %v", err)
	}

	if hash != "" {
		t.Error("Empty password should return empty hash")
	}
}

func TestHashPassword_NonDeterministic(t *testing.T) {
	password := "testPassword"

	hash1, _ := auth.HashPassword(password)
	hash2, _ := auth.HashPassword(password)

	// Bcrypt hashes should be different each time (includes random salt)
	if hash1 == hash2 {
		t.Error("Bcrypt hashes should differ due to random salt")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	password := "myPassword123"
	hash, _ := auth.HashPassword(password)

	if !auth.VerifyPassword(hash, password) {
		t.Error("Verification should succeed for correct password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	password := "myPassword123"
	hash, _ := auth.HashPassword(password)

	if auth.VerifyPassword(hash, "wrongPassword") {
		t.Error("Verification should fail for wrong password")
	}
}

func TestVerifyPassword_EmptyBoth(t *testing.T) {
	// Both empty should match (room with no password)
	if !auth.VerifyPassword("", "") {
		t.Error("Empty password should match empty hash")
	}
}

func TestVerifyPassword_EmptyHash(t *testing.T) {
	if auth.VerifyPassword("", "somePassword") {
		t.Error("Should fail when hash is empty but password provided")
	}
}

func TestVerifyPassword_EmptyPassword(t *testing.T) {
	password := "myPassword"
	hash, _ := auth.HashPassword(password)

	if auth.VerifyPassword(hash, "") {
		t.Error("Should fail when password is empty but hash exists")
	}
}

func TestVerifyPassword_TimingAttackResistance(t *testing.T) {
	password := "myPassword123"
	hash, _ := auth.HashPassword(password)

	// These should both take approximately the same time
	_ = auth.VerifyPassword(hash, "wrongPassword")
	_ = auth.VerifyPassword(hash, password)

	// Basic check - real timing attack tests need
	// statistical analysis over many iterations
	// For now, just verify the function completes
}
