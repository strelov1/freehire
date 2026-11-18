// Package auth holds the user-auth primitives: password hashing, stateless JWT
// issue/verify, and the Fiber middleware that guards protected routes.
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of plain; salt and cost are embedded.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword returns nil when plain matches the bcrypt hash, an error otherwise.
func CheckPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
