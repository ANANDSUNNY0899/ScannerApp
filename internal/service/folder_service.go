package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
)

type FolderService interface {
	CreateFolder(ctx context.Context, userID uuid.UUID, name string) (*model.Folder, error)
	ListFolders(ctx context.Context, userID uuid.UUID) ([]*model.Folder, error)
	GetFolder(ctx context.Context, userID uuid.UUID, id string) (*model.Folder, error)
	DeleteFolder(ctx context.Context, userID uuid.UUID, id string) error
}

type folderService struct {
	folderRepo postgres.FolderRepository
}

func NewFolderService(folderRepo postgres.FolderRepository) FolderService {
	return &folderService{folderRepo: folderRepo}
}

func (s *folderService) CreateFolder(ctx context.Context, userID uuid.UUID, name string) (*model.Folder, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}

	folder := &model.Folder{
		ID:           uuid.New().String(),
		UserID:       userID,
		Name:         trimmedName,
		CreatedAt:    time.Now().UnixMilli(),
		ReceiptCount: 0,
		TotalSpend:   0.0,
	}

	if err := s.folderRepo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return folder, nil
}

func (s *folderService) ListFolders(ctx context.Context, userID uuid.UUID) ([]*model.Folder, error) {
	return s.folderRepo.ListFolders(ctx, userID)
}

func (s *folderService) GetFolder(ctx context.Context, userID uuid.UUID, id string) (*model.Folder, error) {
	return s.folderRepo.GetFolderByID(ctx, userID, id)
}

func (s *folderService) DeleteFolder(ctx context.Context, userID uuid.UUID, id string) error {
	return s.folderRepo.DeleteFolder(ctx, userID, id)
}