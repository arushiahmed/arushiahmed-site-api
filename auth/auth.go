// Package auth provides password hashing and short-lived bearer tokens for
// gating access to protected content, without any session store or database.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of password, suitable for storing in an
// env var. Use the cmd/hashpassword tool to generate one.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IssueToken returns a signed token that's valid until ttl from now. The
// token is "<expiryUnixSeconds>.<base64url hmac signature>" — stateless, no
// server-side storage needed to verify it later.
func IssueToken(secret []byte, ttl time.Duration) string {
	expiry := time.Now().Add(ttl).Unix()
	payload := strconv.FormatInt(expiry, 10)
	sig := sign(secret, payload)
	return payload + "." + sig
}

// VerifyToken reports whether token was signed by secret and hasn't expired.
func VerifyToken(secret []byte, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, sig := parts[0], parts[1]

	expected := sign(secret, payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return false
	}

	expiry, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < expiry
}

func sign(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Require wraps next so it only runs when the request carries a valid
// "Authorization: Bearer <token>" header signed by secret.
func Require(secret []byte, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || !VerifyToken(secret, token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
