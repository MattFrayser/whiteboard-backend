package auth

import (
	"crypto/subtle"
	"golang.org/x/crypto/bcrypt"
)

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

// provided password matches the hashed password
func VerifyPassword(hashedPassword, password string) bool {
	// Empty password check
	if hashedPassword == "" && password == "" {
		return true
	}

	if hashedPassword == "" || password == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))

	// constant-time comparison 
	// comapare (ok: 1/err: 0) to contant one
	return subtle.ConstantTimeCompare([]byte{boolToByte(err == nil)}, []byte{1}) == 1
}

// if err == nil then ok -> return 1
// err != return 0 and fail compare
func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
