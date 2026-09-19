package model

import (
	"github.com/google/uuid"
)

// Folder represents a project workspace folder that isolates receipt ledgers and documents.
type Folder struct {
	ID           string    `json:"id" db:"id"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	Name         string    `json:"name" db:"name"`
	CreatedAt    int64     `json:"created_at" db:"created_at"`
	ReceiptCount int       `json:"receipt_count" db:"receipt_count"`
	TotalSpend   float64   `json:"total_spend" db:"total_spend"`
}

// CreateFolderRequest holds the payload for creating a new workspace folder.
type CreateFolderRequest struct {
	Name string `json:"name"`
}