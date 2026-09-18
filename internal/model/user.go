package model

import (
	"time"

	"github.com/google/uuid"
)

type UserTier string

const (
	TierFree    UserTier = "free"
	TierPremium UserTier = "premium"
)

type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Tier         UserTier  `json:"tier" db:"tier"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
	User      User   `json:"user"`
}
