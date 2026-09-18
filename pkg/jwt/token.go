package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type CustomClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Tier   string    `json:"tier"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secretKey     []byte
	tokenDuration time.Duration
}

func NewTokenManager(secretKey string, durationHours int) *TokenManager {
	return &TokenManager{
		secretKey:     []byte(secretKey),
		tokenDuration: time.Duration(durationHours) * time.Hour,
	}
}

func (tm *TokenManager) GenerateToken(userID uuid.UUID, email, tier string) (string, int64, error) {
	expirationTime := time.Now().Add(tm.tokenDuration)
	claims := &CustomClaims{
		UserID: userID,
		Email:  email,
		Tier:   tier,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(tm.secretKey)
	if err != nil {
		return "", 0, err
	}

	return tokenString, int64(tm.tokenDuration.Seconds()), nil
}

func (tm *TokenManager) ValidateToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return tm.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
