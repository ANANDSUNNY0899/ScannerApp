package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/storage"
)

type ReceiptService interface {
	ExtractAndSaveReceipt(ctx context.Context, userID uuid.UUID, fileReader io.Reader, fileSize int64, fileName, mimeType string, folderID *string) (*model.ReceiptOrder, error)
	GetReceipt(ctx context.Context, userID, id uuid.UUID) (*model.ReceiptOrder, error)
	ListReceipts(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]*model.ReceiptOrder, error)
	UpdateReceipt(ctx context.Context, userID, id uuid.UUID, req *model.UpdateReceiptRequest) (*model.ReceiptOrder, error)
	DeleteReceipt(ctx context.Context, userID, id uuid.UUID) error
	GetSummary(ctx context.Context, userID uuid.UUID, startDate, endDate, folderID string) (*model.ReceiptSummary, error)
	GenerateLedgerCSV(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]byte, error)
}

type receiptService struct {
	receiptRepo   postgres.ReceiptRepository
	geminiService GeminiService
	s3Storage     storage.StorageService
	bucketName    string
}

func NewReceiptService(
	receiptRepo postgres.ReceiptRepository,
	geminiService GeminiService,
	s3Storage storage.StorageService,
	bucketName string,
) ReceiptService {
	return &receiptService{
		receiptRepo:   receiptRepo,
		geminiService: geminiService,
		s3Storage:     s3Storage,
		bucketName:    bucketName,
	}
}

func (s *receiptService) ExtractAndSaveReceipt(
	ctx context.Context,
	userID uuid.UUID,
	fileReader io.Reader,
	fileSize int64,
	fileName, mimeType string,
	folderID *string,
) (*model.ReceiptOrder, error) {
	if fileReader == nil {
		return nil, fmt.Errorf("file reader cannot be nil")
	}
	if s.geminiService == nil {
		return nil, fmt.Errorf("gemini service is not configured")
	}
	if s.receiptRepo == nil {
		return nil, fmt.Errorf("receipt repository is not initialized")
	}

	// Read full image bytes into memory for Gemini & S3 upload
	imageBytes, err := io.ReadAll(fileReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read receipt image data: %w", err)
	}
	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("uploaded receipt image data is empty")
	}

	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// 1. Upload to Object Storage (S3 / MinIO) if storage is available
	imageURL := ""
	if s.s3Storage != nil {
		objectKey := fmt.Sprintf("receipts/%s/%d_%s", userID.String(), time.Now().UnixNano(), fileName)
		uploadedKey, err := s.s3Storage.Upload(ctx, s.bucketName, objectKey, bytes.NewReader(imageBytes), int64(len(imageBytes)), mimeType)
		if err == nil {
			imageURL = uploadedKey
		}
	}

	// 2. Extract structured JSON data via Google Gemini Multimodal
	extracted, err := s.geminiService.ExtractReceiptData(ctx, imageBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to extract receipt with gemini: %w", err)
	}
	if extracted == nil {
		return nil, fmt.Errorf("gemini extraction returned empty data")
	}

	vendorName := extracted.VendorName
	if vendorName == "" {
		vendorName = "Unknown Vendor"
	}
	category := extracted.Category
	if category == "" {
		category = "Other"
	}
	orderDate := extracted.OrderDate
	if orderDate == "" {
		orderDate = time.Now().Format("2006-01-02")
	}

	// 3. Persist into PostgreSQL receipt_orders table
	order := &model.ReceiptOrder{
		ID:          uuid.New(),
		UserID:      userID,
		VendorName:  vendorName,
		Category:    category,
		Description: extracted.Description,
		TotalPrice:  extracted.TotalPrice,
		OrderDate:   orderDate,
		ImageURL:    imageURL,
		FolderID:    folderID,
	}

	if err := s.receiptRepo.CreateReceiptOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to save receipt order to database: %w", err)
	}

	return order, nil
}

func (s *receiptService) GetReceipt(ctx context.Context, userID, id uuid.UUID) (*model.ReceiptOrder, error) {
	return s.receiptRepo.GetReceiptOrderByID(ctx, userID, id)
}

func (s *receiptService) ListReceipts(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]*model.ReceiptOrder, error) {
	return s.receiptRepo.ListReceiptOrders(ctx, userID, filter)
}

func (s *receiptService) UpdateReceipt(ctx context.Context, userID, id uuid.UUID, req *model.UpdateReceiptRequest) (*model.ReceiptOrder, error) {
	return s.receiptRepo.UpdateReceiptOrder(ctx, userID, id, req)
}

func (s *receiptService) DeleteReceipt(ctx context.Context, userID, id uuid.UUID) error {
	return s.receiptRepo.DeleteReceiptOrder(ctx, userID, id)
}

func (s *receiptService) GetSummary(ctx context.Context, userID uuid.UUID, startDate, endDate, folderID string) (*model.ReceiptSummary, error) {
	return s.receiptRepo.GetReceiptSummary(ctx, userID, startDate, endDate, folderID)
}

// GenerateLedgerCSV generates a clean CSV ledger stream compatible with Microsoft Excel and Google Sheets
func (s *receiptService) GenerateLedgerCSV(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]byte, error) {
	// Set unlimited for export
	filter.Limit = 10000
	filter.Offset = 0

	orders, err := s.receiptRepo.ListReceiptOrders(ctx, userID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve receipts for export: %w", err)
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header row: essential organic columns for business partners
	header := []string{
		"Date",
		"Vendor",
		"Description",
		"Total Price",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	// Data rows
	for _, order := range orders {
		row := []string{
			order.OrderDate,
			order.VendorName,
			order.Description,
			fmt.Sprintf("%.2f", order.TotalPrice),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("csv writing error: %w", err)
	}

	return buf.Bytes(), nil
}
