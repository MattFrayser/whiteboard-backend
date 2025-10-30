package auth

import (
	"crypto/subtle"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword creates a bcrypt hash of the password
// Returns empty string if password is empty
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword checks if the provided password matches the hashed password
// Uses constant-time comparison to prevent timing attacks
func VerifyPassword(hashedPassword, password string) bool {
	// Empty password check
	if hashedPassword == "" && password == "" {
		return true
	}

	if hashedPassword == "" || password == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte{boolToByte(err == nil)}, []byte{1}) == 1
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
