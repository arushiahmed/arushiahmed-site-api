package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHashPassword_CheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if !CheckPassword(hash, "correct-horse") {
		t.Error("CheckPassword should accept the correct password")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword should reject an incorrect password")
	}
}

func TestIssueToken_VerifyToken(t *testing.T) {
	secret := []byte("test-secret")

	token := IssueToken(secret, time.Hour)
	if !VerifyToken(secret, token) {
		t.Error("VerifyToken should accept a freshly issued token")
	}
}

func TestVerifyToken_RejectsExpired(t *testing.T) {
	secret := []byte("test-secret")

	token := IssueToken(secret, -time.Hour) // already expired
	if VerifyToken(secret, token) {
		t.Error("VerifyToken should reject an expired token")
	}
}

func TestVerifyToken_RejectsWrongSecret(t *testing.T) {
	token := IssueToken([]byte("secret-a"), time.Hour)
	if VerifyToken([]byte("secret-b"), token) {
		t.Error("VerifyToken should reject a token signed with a different secret")
	}
}

func TestVerifyToken_RejectsMalformed(t *testing.T) {
	secret := []byte("test-secret")
	for _, token := range []string{"", "no-dot-here", "expiry.wrong-sig.extra"} {
		if VerifyToken(secret, token) {
			t.Errorf("VerifyToken should reject malformed token %q", token)
		}
	}
}

func TestRequire(t *testing.T) {
	secret := []byte("test-secret")
	inner := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	handler := Require(secret, inner)

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+IssueToken(secret, time.Hour))
		rec := httptest.NewRecorder()

		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rec := httptest.NewRecorder()

		handler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}
