package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
)

type SyncRepository interface {
	UpsertFolders(ctx context.Context, userID uuid.UUID, folders []model.FolderSync) error
	UpsertDocuments(ctx context.Context, userID uuid.UUID, documents []model.DocumentSync) error
	UpsertPages(ctx context.Context, pages []model.PageSync) error
	GetUserData(ctx context.Context, userID uuid.UUID, since int64) (*model.SyncPullResponse, error)
	UpdateDocumentFile(ctx context.Context, userID uuid.UUID, docID string, fileURL string) error
	GetDocument(ctx context.Context, userID uuid.UUID, docID string) (*model.DocumentSync, error)
}

type syncRepo struct {
	db *DB
}

func NewSyncRepository(db *DB) SyncRepository {
	return &syncRepo{db: db}
}

func (r *syncRepo) UpsertFolders(ctx context.Context, userID uuid.UUID, folders []model.FolderSync) error {
	if len(folders) == 0 {
		return nil
	}

	// LWW check: only update if incoming client_updated_at is strictly newer
	query := `
		INSERT INTO folders (id, user_id, name, is_locked, created_at, updated_at, client_updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    is_locked = EXCLUDED.is_locked,
		    updated_at = EXCLUDED.updated_at,
		    client_updated_at = EXCLUDED.client_updated_at,
		    deleted_at = EXCLUDED.deleted_at
		WHERE EXCLUDED.client_updated_at > folders.client_updated_at OR folders.client_updated_at IS NULL;
	`
	now := time.Now()
	for _, f := range folders {
		clientUpdatedAt := f.ClientUpdatedAt
		if clientUpdatedAt.IsZero() {
			clientUpdatedAt = now
		}
		_, err := r.db.ExecContext(
			ctx,
			query,
			f.ID,
			userID,
			f.Name,
			f.IsLocked,
			f.CreatedAt,
			now,
			clientUpdatedAt,
			f.DeletedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *syncRepo) UpsertDocuments(ctx context.Context, userID uuid.UUID, documents []model.DocumentSync) error {
	if len(documents) == 0 {
		return nil
	}

	// LWW check: only update if incoming client_updated_at is strictly newer
	query := `
		INSERT INTO documents (id, user_id, folder_id, title, pdf_cloud_url, created_at, updated_at, client_updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE
		SET folder_id = EXCLUDED.folder_id,
		    title = EXCLUDED.title,
		    pdf_cloud_url = COALESCE(EXCLUDED.pdf_cloud_url, documents.pdf_cloud_url),
		    updated_at = EXCLUDED.updated_at,
		    client_updated_at = EXCLUDED.client_updated_at,
		    deleted_at = EXCLUDED.deleted_at
		WHERE EXCLUDED.client_updated_at > documents.client_updated_at OR documents.client_updated_at IS NULL;
	`
	now := time.Now()
	for _, d := range documents {
		clientUpdatedAt := d.ClientUpdatedAt
		if clientUpdatedAt.IsZero() {
			clientUpdatedAt = now
		}
		_, err := r.db.ExecContext(
			ctx,
			query,
			d.ID,
			userID,
			d.FolderID,
			d.Title,
			d.PdfCloudURL,
			d.CreatedAt,
			now,
			clientUpdatedAt,
			d.DeletedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *syncRepo) UpsertPages(ctx context.Context, pages []model.PageSync) error {
	if len(pages) == 0 {
		return nil
	}

	// LWW check: only update if incoming client_updated_at is strictly newer
	query := `
		INSERT INTO pages (id, document_id, page_number, image_cloud_url, extracted_text, created_at, client_updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE
		SET page_number = EXCLUDED.page_number,
		    image_cloud_url = COALESCE(EXCLUDED.image_cloud_url, pages.image_cloud_url),
		    extracted_text = COALESCE(EXCLUDED.extracted_text, pages.extracted_text),
		    client_updated_at = EXCLUDED.client_updated_at,
		    deleted_at = EXCLUDED.deleted_at
		WHERE EXCLUDED.client_updated_at > pages.client_updated_at OR pages.client_updated_at IS NULL;
	`
	now := time.Now()
	for _, p := range pages {
		clientUpdatedAt := p.ClientUpdatedAt
		if clientUpdatedAt.IsZero() {
			clientUpdatedAt = now
		}
		_, err := r.db.ExecContext(
			ctx,
			query,
			p.ID,
			p.DocumentID,
			p.PageNumber,
			p.ImageCloudURL,
			p.ExtractedText,
			now,
			clientUpdatedAt,
			p.DeletedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *syncRepo) GetUserData(ctx context.Context, userID uuid.UUID, since int64) (*model.SyncPullResponse, error) {
	resp := &model.SyncPullResponse{
		Folders:   []model.FolderSync{},
		Documents: []model.DocumentSync{},
		Pages:     []model.PageSync{},
		SyncedAt:  time.Now().Unix(),
	}

	// Fetch Folders (including soft-deleted tombstones)
	var folderQuery string
	var err error
	if since > 0 {
		folderQuery = `
			SELECT id, user_id, name, is_locked, created_at, updated_at, client_updated_at, deleted_at 
			FROM folders 
			WHERE user_id = $1 AND (client_updated_at > to_timestamp($2) OR updated_at > to_timestamp($2));
		`
		err = r.db.SelectContext(ctx, &resp.Folders, folderQuery, userID, since)
	} else {
		folderQuery = `
			SELECT id, user_id, name, is_locked, created_at, updated_at, client_updated_at, deleted_at 
			FROM folders 
			WHERE user_id = $1;
		`
		err = r.db.SelectContext(ctx, &resp.Folders, folderQuery, userID)
	}
	if err != nil {
		return nil, err
	}

	// Fetch Documents (including tombstones)
	var docQuery string
	if since > 0 {
		docQuery = `
			SELECT id, user_id, folder_id, title, pdf_cloud_url, created_at, updated_at, client_updated_at, deleted_at 
			FROM documents 
			WHERE user_id = $1 AND (client_updated_at > to_timestamp($2) OR updated_at > to_timestamp($2));
		`
		err = r.db.SelectContext(ctx, &resp.Documents, docQuery, userID, since)
	} else {
		docQuery = `
			SELECT id, user_id, folder_id, title, pdf_cloud_url, created_at, updated_at, client_updated_at, deleted_at 
			FROM documents 
			WHERE user_id = $1;
		`
		err = r.db.SelectContext(ctx, &resp.Documents, docQuery, userID)
	}
	if err != nil {
		return nil, err
	}

	// Fetch Pages (including tombstones)
	var pageQuery string
	if since > 0 {
		pageQuery = `
			SELECT p.id, p.document_id, p.page_number, p.image_cloud_url, p.extracted_text, p.created_at, p.client_updated_at, p.deleted_at
			FROM pages p
			JOIN documents d ON p.document_id = d.id
			WHERE d.user_id = $1 AND (p.client_updated_at > to_timestamp($2) OR p.created_at > to_timestamp($2));
		`
		err = r.db.SelectContext(ctx, &resp.Pages, pageQuery, userID, since)
	} else {
		pageQuery = `
			SELECT p.id, p.document_id, p.page_number, p.image_cloud_url, p.extracted_text, p.created_at, p.client_updated_at, p.deleted_at
			FROM pages p
			JOIN documents d ON p.document_id = d.id
			WHERE d.user_id = $1;
		`
		err = r.db.SelectContext(ctx, &resp.Pages, pageQuery, userID)
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *syncRepo) UpdateDocumentFile(ctx context.Context, userID uuid.UUID, docID string, fileURL string) error {
	now := time.Now()
	query := `
		UPDATE documents
		SET pdf_cloud_url = $1, updated_at = $2, client_updated_at = $2
		WHERE id = $3 AND user_id = $4;
	`
	_, err := r.db.ExecContext(ctx, query, fileURL, now, docID, userID)
	return err
}

func (r *syncRepo) GetDocument(ctx context.Context, userID uuid.UUID, docID string) (*model.DocumentSync, error) {
	query := `
		SELECT id, user_id, folder_id, title, pdf_cloud_url, created_at, updated_at, client_updated_at, deleted_at
		FROM documents
		WHERE id = $1 AND user_id = $2;
	`
	var doc model.DocumentSync
	err := r.db.GetContext(ctx, &doc, query, docID, userID)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}
