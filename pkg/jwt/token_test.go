package jwt_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/pkg/jwt"
)

func TestTokenGenerationAndValidation(t *testing.T) {
	secret := "test-secret-key-12345"
	tm := jwt.NewTokenManager(secret, 1)

	userID := uuid.New()
	email := "user@example.com"
	tier := "free"

	token, exp, err := tm.GenerateToken(userID, email, tier)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	if exp <= 0 {
		t.Fatalf("expected positive expiration, got %d", exp)
	}

	claims, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, claims.UserID)
	}

	if claims.Email != email {
		t.Errorf("expected email %s, got %s", email, claims.Email)
	}

	if claims.Tier != tier {
		t.Errorf("expected tier %s, got %s", tier, claims.Tier)
	}
}

func TestInvalidTokenValidation(t *testing.T) {
	tm := jwt.NewTokenManager("secret-1", 1)
	otherTM := jwt.NewTokenManager("secret-2", 1)

	token, _, _ := tm.GenerateToken(uuid.New(), "test@example.com", "free")

	_, err := otherTM.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error validating token with mismatched secret, got nil")
	}
}
