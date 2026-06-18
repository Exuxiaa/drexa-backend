package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const defaultCost = bcrypt.DefaultCost

// Hash returns the bcrypt hash of the plaintext password.
func Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), defaultCost)
	if err != nil {
		return "", fmt.Errorf("password: hash: %w", err)
	}
	return string(h), nil
}

// Check returns nil if plain matches the stored hash, bcrypt.ErrMismatchedHashAndPassword otherwise.
func Check(plain, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
