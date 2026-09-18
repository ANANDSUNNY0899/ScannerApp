package model

import (
	"time"

	"github.com/google/uuid"
)

type FolderSync struct {
	ID              string     `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	Name            string     `json:"name" db:"name"`
	IsLocked        bool       `json:"is_locked" db:"is_locked"`
	CreatedAt       int64      `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	ClientUpdatedAt time.Time  `json:"client_updated_at" db:"client_updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type DocumentSync struct {
	ID              string     `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	FolderID        *string    `json:"folder_id" db:"folder_id"`
	Title           string     `json:"title" db:"title"`
	PdfCloudURL     *string    `json:"pdf_cloud_url" db:"pdf_cloud_url"`
	CreatedAt       int64      `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	ClientUpdatedAt time.Time  `json:"client_updated_at" db:"client_updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	Pages           []PageSync `json:"pages,omitempty"`
}

type PageSync struct {
	ID              string     `json:"id" db:"id"`
	DocumentID      string     `json:"document_id" db:"document_id"`
	PageNumber      int        `json:"page_number" db:"page_number"`
	ImageCloudURL   *string    `json:"image_cloud_url" db:"image_cloud_url"`
	ExtractedText   *string    `json:"extracted_text" db:"extracted_text"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	ClientUpdatedAt time.Time  `json:"client_updated_at" db:"client_updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type SyncPushRequest struct {
	Folders   []FolderSync   `json:"folders"`
	Documents []DocumentSync `json:"documents"`
	Pages     []PageSync     `json:"pages"`
}

type SyncPullResponse struct {
	Folders   []FolderSync   `json:"folders"`
	Documents []DocumentSync `json:"documents"`
	Pages     []PageSync     `json:"pages"`
	SyncedAt  int64          `json:"synced_at"`
}
