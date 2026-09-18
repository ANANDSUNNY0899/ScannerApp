package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
)

type SyncService interface {
	PushData(ctx context.Context, userID uuid.UUID, req *model.SyncPushRequest) error
	PullData(ctx context.Context, userID uuid.UUID, since int64) (*model.SyncPullResponse, error)
	UpdateDocumentFile(ctx context.Context, userID uuid.UUID, docID string, fileURL string) error
	GetDocument(ctx context.Context, userID uuid.UUID, docID string) (*model.DocumentSync, error)
}

type syncService struct {
	syncRepo postgres.SyncRepository
}

func NewSyncService(syncRepo postgres.SyncRepository) SyncService {
	return &syncService{syncRepo: syncRepo}
}

func (s *syncService) PushData(ctx context.Context, userID uuid.UUID, req *model.SyncPushRequest) error {
	if err := s.syncRepo.UpsertFolders(ctx, userID, req.Folders); err != nil {
		return err
	}
	if err := s.syncRepo.UpsertDocuments(ctx, userID, req.Documents); err != nil {
		return err
	}
	if err := s.syncRepo.UpsertPages(ctx, req.Pages); err != nil {
		return err
	}
	return nil
}

func (s *syncService) PullData(ctx context.Context, userID uuid.UUID, since int64) (*model.SyncPullResponse, error) {
	return s.syncRepo.GetUserData(ctx, userID, since)
}

func (s *syncService) UpdateDocumentFile(ctx context.Context, userID uuid.UUID, docID string, fileURL string) error {
	return s.syncRepo.UpdateDocumentFile(ctx, userID, docID, fileURL)
}

func (s *syncService) GetDocument(ctx context.Context, userID uuid.UUID, docID string) (*model.DocumentSync, error) {
	return s.syncRepo.GetDocument(ctx, userID, docID)
}
