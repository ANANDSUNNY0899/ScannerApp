package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserAlreadyExists = errors.New("a user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound      = errors.New("user not found")
)

type AuthService interface {
	Register(ctx context.Context, email, password string) (*model.AuthResponse, error)
	Login(ctx context.Context, email, password string) (*model.AuthResponse, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*model.User, error)
}

type authService struct {
	userRepo     postgres.UserRepository
	tokenManager *jwt.TokenManager
}

func NewAuthService(userRepo postgres.UserRepository, tokenManager *jwt.TokenManager) AuthService {
	return &authService{
		userRepo:     userRepo,
		tokenManager: tokenManager,
	}
}

func (s *authService) Register(ctx context.Context, email, password string) (*model.AuthResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	existing, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.CreateUser(ctx, email, string(hash))
	if err != nil {
		return nil, err
	}

	token, exp, err := s.tokenManager.GenerateToken(user.ID, user.Email, string(user.Tier))
	if err != nil {
		return nil, err
	}

	return &model.AuthResponse{
		Token:     token,
		ExpiresIn: exp,
		User:      *user,
	}, nil
}

func (s *authService) Login(ctx context.Context, email, password string) (*model.AuthResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, exp, err := s.tokenManager.GenerateToken(user.ID, user.Email, string(user.Tier))
	if err != nil {
		return nil, err
	}

	return &model.AuthResponse{
		Token:     token,
		ExpiresIn: exp,
		User:      *user,
	}, nil
}

func (s *authService) GetProfile(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}
