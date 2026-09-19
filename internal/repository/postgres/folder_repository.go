package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
)

var (
	ErrFolderNotFound = errors.New("folder not found")
)

type FolderRepository interface {
	CreateFolder(ctx context.Context, folder *model.Folder) error
	ListFolders(ctx context.Context, userID uuid.UUID) ([]*model.Folder, error)
	GetFolderByID(ctx context.Context, userID uuid.UUID, id string) (*model.Folder, error)
	DeleteFolder(ctx context.Context, userID uuid.UUID, id string) error
}

type folderRepository struct {
	db *DB
}

func NewFolderRepository(db *DB) FolderRepository {
	return &folderRepository{db: db}
}

func (r *folderRepository) CreateFolder(ctx context.Context, folder *model.Folder) error {
	if folder.ID == "" {
		folder.ID = uuid.New().String()
	}
	if folder.CreatedAt == 0 {
		folder.CreatedAt = time.Now().UnixMilli()
	}

	query := `
		INSERT INTO folders (id, user_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err := r.db.ExecContext(ctx, query, folder.ID, folder.UserID, folder.Name, folder.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}
	return nil
}

func (r *folderRepository) ListFolders(ctx context.Context, userID uuid.UUID) ([]*model.Folder, error) {
	query := `
		SELECT 
			f.id,
			f.user_id,
			f.name,
			f.created_at,
			COUNT(r.id) AS receipt_count,
			COALESCE(SUM(r.total_price), 0.0) AS total_spend
		FROM folders f
		LEFT JOIN receipt_orders r ON r.folder_id = f.id AND r.user_id = f.user_id
		WHERE f.user_id = $1 AND (f.deleted_at IS NULL)
		GROUP BY f.id, f.user_id, f.name, f.created_at
		ORDER BY f.created_at DESC
	`

	folders := make([]*model.Folder, 0)
	err := r.db.SelectContext(ctx, &folders, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	if folders == nil {
		folders = make([]*model.Folder, 0)
	}
	return folders, nil
}

func (r *folderRepository) GetFolderByID(ctx context.Context, userID uuid.UUID, id string) (*model.Folder, error) {
	query := `
		SELECT 
			f.id,
			f.user_id,
			f.name,
			f.created_at,
			COUNT(r.id) AS receipt_count,
			COALESCE(SUM(r.total_price), 0.0) AS total_spend
		FROM folders f
		LEFT JOIN receipt_orders r ON r.folder_id = f.id AND r.user_id = f.user_id
		WHERE f.id = $1 AND f.user_id = $2 AND (f.deleted_at IS NULL)
		GROUP BY f.id, f.user_id, f.name, f.created_at
	`

	var folder model.Folder
	err := r.db.GetContext(ctx, &folder, query, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	return &folder, nil
}

func (r *folderRepository) DeleteFolder(ctx context.Context, userID uuid.UUID, id string) error {
	query := `UPDATE folders SET deleted_at = NOW() WHERE id = $1 AND user_id = $2`
	res, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return ErrFolderNotFound
	}
	return nil
}
