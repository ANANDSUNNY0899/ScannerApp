package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
)

type mockReceiptRepo struct {
	orders []*model.ReceiptOrder
}

func (m *mockReceiptRepo) CreateReceiptOrder(ctx context.Context, order *model.ReceiptOrder) error {
	m.orders = append(m.orders, order)
	return nil
}

func (m *mockReceiptRepo) GetReceiptOrderByID(ctx context.Context, userID, id uuid.UUID) (*model.ReceiptOrder, error) {
	for _, o := range m.orders {
		if o.ID == id && o.UserID == userID {
			return o, nil
		}
	}
	return nil, nil
}

func (m *mockReceiptRepo) ListReceiptOrders(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]*model.ReceiptOrder, error) {
	var results []*model.ReceiptOrder
	for _, o := range m.orders {
		if o.UserID == userID {
			results = append(results, o)
		}
	}
	return results, nil
}

func (m *mockReceiptRepo) UpdateReceiptOrder(ctx context.Context, userID, id uuid.UUID, req *model.UpdateReceiptRequest) (*model.ReceiptOrder, error) {
	return nil, nil
}

func (m *mockReceiptRepo) DeleteReceiptOrder(ctx context.Context, userID, id uuid.UUID) error {
	return nil
}

func (m *mockReceiptRepo) GetReceiptSummary(ctx context.Context, userID uuid.UUID, startDate, endDate string) (*model.ReceiptSummary, error) {
	return &model.ReceiptSummary{}, nil
}

func TestGenerateLedgerCSV(t *testing.T) {
	userID := uuid.New()
	repo := &mockReceiptRepo{
		orders: []*model.ReceiptOrder{
			{
				ID:          uuid.New(),
				UserID:      userID,
				VendorName:  "Acme Hardware",
				Category:    "Construction",
				Description: "50 Bags Cement",
				TotalPrice:  225.50,
				OrderDate:   "2026-09-15",
				ImageURL:    "https://storage.example.com/receipt1.jpg",
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				UserID:      userID,
				VendorName:  "Bistro 42",
				Category:    "Dining",
				Description: "Lunch meeting",
				TotalPrice:  42.00,
				OrderDate:   "2026-09-16",
				ImageURL:    "https://storage.example.com/receipt2.jpg",
				CreatedAt:   time.Now(),
			},
		},
	}

	svc := NewReceiptService(repo, nil, nil, "test-bucket")
	csvBytes, err := svc.GenerateLedgerCSV(context.Background(), userID, model.ReceiptFilter{})
	if err != nil {
		t.Fatalf("failed to generate ledger CSV: %v", err)
	}

	csvStr := string(csvBytes)

	// Check Header
	if !strings.Contains(csvStr, "Date,Vendor,Description,Total Price") {
		t.Errorf("CSV missing expected header row. Got:\n%s", csvStr)
	}

	// Check Data Rows
	if !strings.Contains(csvStr, "Acme Hardware") || !strings.Contains(csvStr, "225.50") {
		t.Errorf("CSV missing Acme Hardware record. Got:\n%s", csvStr)
	}

	if !strings.Contains(csvStr, "Bistro 42") || !strings.Contains(csvStr, "42.00") {
		t.Errorf("CSV missing Bistro 42 record. Got:\n%s", csvStr)
	}
}
