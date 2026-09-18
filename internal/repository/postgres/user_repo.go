package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
)

type UserRepository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UpdateUserTier(ctx context.Context, id uuid.UUID, tier model.UserTier) error
}

type userRepo struct {
	db *DB
}

func NewUserRepository(db *DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) CreateUser(ctx context.Context, email, passwordHash string) (*model.User, error) {
	query := `
		INSERT INTO users (email, password_hash, tier, created_at, updated_at)
		VALUES ($1, $2, 'free', $3, $3)
		RETURNING id, email, password_hash, tier, created_at, updated_at;
	`
	now := time.Now()
	var user model.User
	err := r.db.QueryRowxContext(ctx, query, email, passwordHash, now).StructScan(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, email, password_hash, tier, created_at, updated_at FROM users WHERE email = $1;`
	var user model.User
	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `SELECT id, email, password_hash, tier, created_at, updated_at FROM users WHERE id = $1;`
	var user model.User
	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) UpdateUserTier(ctx context.Context, id uuid.UUID, tier model.UserTier) error {
	query := `UPDATE users SET tier = $1, updated_at = $2 WHERE id = $3;`
	_, err := r.db.ExecContext(ctx, query, tier, time.Now(), id)
	return err
}
