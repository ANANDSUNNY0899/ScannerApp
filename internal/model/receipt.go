package model

import (
	"time"

	"github.com/google/uuid"
)

// ReceiptOrder represents an extracted and stored receipt record in PostgreSQL.
type ReceiptOrder struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	VendorName  string    `json:"vendor_name" db:"vendor_name"`
	Category    string    `json:"category" db:"category"`
	Description string    `json:"description" db:"description"`
	TotalPrice  float64   `json:"total_price" db:"total_price"`
	OrderDate   string    `json:"order_date" db:"order_date"` // YYYY-MM-DD
	ImageURL    string    `json:"image_url" db:"image_url"`
	FolderID    *string   `json:"folder_id" db:"folder_id"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// ReceiptExtractedData represents the structured JSON output from the Gemini Multimodal model.
type ReceiptExtractedData struct {
	VendorName  string  `json:"vendor_name"`
	OrderDate   string  `json:"order_date"` // YYYY-MM-DD
	Category    string  `json:"category"`
	Description string  `json:"description"`
	TotalPrice  float64 `json:"total_price"`
	Confidence  float64 `json:"confidence,omitempty"`
}

// ReceiptFilter encapsulates query filters for listing and Excel/CSV ledger export.
type ReceiptFilter struct {
	FolderID  string `json:"folder_id"`
	Vendor    string `json:"vendor"`
	Category  string `json:"category"`
	StartDate string `json:"start_date"` // YYYY-MM-DD
	EndDate   string `json:"end_date"`   // YYYY-MM-DD
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// UpdateReceiptRequest contains optional update parameters for an existing receipt.
type UpdateReceiptRequest struct {
	VendorName  *string  `json:"vendor_name"`
	Category    *string  `json:"category"`
	Description *string  `json:"description"`
	TotalPrice  *float64 `json:"total_price"`
	OrderDate   *string  `json:"order_date"`
	FolderID    *string  `json:"folder_id"`
}

// CategoryExpenseSummary aggregates total spend for a given category.
type CategoryExpenseSummary struct {
	Category   string  `json:"category" db:"category"`
	TotalSpend float64 `json:"total_spend" db:"total_spend"`
	Count      int     `json:"count" db:"count"`
}

// ReceiptSummary provides high-level financial aggregations for the user's ledger.
type ReceiptSummary struct {
	TotalSpend float64                  `json:"total_spend"`
	TotalCount int                      `json:"total_count"`
	Categories []CategoryExpenseSummary `json:"categories"`
}
