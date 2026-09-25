package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
)

var (
	ErrReceiptNotFound = errors.New("receipt order not found")
)

type ReceiptRepository interface {
	CreateReceiptOrder(ctx context.Context, order *model.ReceiptOrder) error
	GetReceiptOrderByID(ctx context.Context, userID, id uuid.UUID) (*model.ReceiptOrder, error)
	ListReceiptOrders(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]*model.ReceiptOrder, error)
	UpdateReceiptOrder(ctx context.Context, userID, id uuid.UUID, req *model.UpdateReceiptRequest) (*model.ReceiptOrder, error)
	DeleteReceiptOrder(ctx context.Context, userID, id uuid.UUID) error
	GetReceiptSummary(ctx context.Context, userID uuid.UUID, startDate, endDate, folderID string) (*model.ReceiptSummary, error)
}

type receiptRepository struct {
	db *DB
}

func NewReceiptRepository(db *DB) ReceiptRepository {
	return &receiptRepository{db: db}
}

func (r *receiptRepository) CreateReceiptOrder(ctx context.Context, order *model.ReceiptOrder) error {
	query := `
		INSERT INTO receipt_orders (
			id, user_id, vendor_name, category, description, total_price, order_date, image_url, folder_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW()
		) RETURNING created_at
	`
	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}

	err := r.db.QueryRowContext(
		ctx,
		query,
		order.ID,
		order.UserID,
		order.VendorName,
		order.Category,
		order.Description,
		order.TotalPrice,
		order.OrderDate,
		order.ImageURL,
		order.FolderID,
	).Scan(&order.CreatedAt)

	// If foreign key constraint violation on folder_id (e.g. folder hasn't synced yet from client),
	// retry without the folder foreign key so the user's receipt is never lost or dropped.
	if err != nil && order.FolderID != nil && strings.Contains(strings.ToLower(err.Error()), "folder_id") {
		order.FolderID = nil
		return r.db.QueryRowContext(
			ctx,
			query,
			order.ID,
			order.UserID,
			order.VendorName,
			order.Category,
			order.Description,
			order.TotalPrice,
			order.OrderDate,
			order.ImageURL,
			nil,
		).Scan(&order.CreatedAt)
	}

	return err
}

func (r *receiptRepository) GetReceiptOrderByID(ctx context.Context, userID, id uuid.UUID) (*model.ReceiptOrder, error) {
	query := `
		SELECT id, user_id, vendor_name, category, description, total_price, 
		       COALESCE(TO_CHAR(order_date, 'YYYY-MM-DD'), '') AS order_date, 
		       image_url, folder_id, created_at
		FROM receipt_orders
		WHERE id = $1 AND user_id = $2
	`
	var order model.ReceiptOrder
	err := r.db.GetContext(ctx, &order, query, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrReceiptNotFound
		}
		return nil, fmt.Errorf("failed to get receipt order: %w", err)
	}
	return &order, nil
}

func (r *receiptRepository) ListReceiptOrders(ctx context.Context, userID uuid.UUID, filter model.ReceiptFilter) ([]*model.ReceiptOrder, error) {
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
		SELECT id, user_id, vendor_name, category, description, total_price, 
		       COALESCE(TO_CHAR(order_date, 'YYYY-MM-DD'), '') AS order_date, 
		       image_url, folder_id, created_at
		FROM receipt_orders
		WHERE user_id = $1
	`)

	args := []interface{}{userID}
	argIdx := 2

	if filter.FolderID != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND folder_id = $%d", argIdx))
		args = append(args, filter.FolderID)
		argIdx++
	}

	if filter.Category != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND category ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Category+"%")
		argIdx++
	}

	if filter.Vendor != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND vendor_name ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Vendor+"%")
		argIdx++
	}

	if filter.StartDate != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND order_date >= $%d::date", argIdx))
		args = append(args, filter.StartDate)
		argIdx++
	}

	if filter.EndDate != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND order_date <= $%d::date", argIdx))
		args = append(args, filter.EndDate)
		argIdx++
	}

	queryBuilder.WriteString(" ORDER BY order_date DESC, created_at DESC")

	limit := 100
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	}
	queryBuilder.WriteString(fmt.Sprintf(" LIMIT $%d", argIdx))
	args = append(args, limit)
	argIdx++

	if filter.Offset > 0 {
		queryBuilder.WriteString(fmt.Sprintf(" OFFSET $%d", argIdx))
		args = append(args, filter.Offset)
	}

	orders := make([]*model.ReceiptOrder, 0)
	err := r.db.SelectContext(ctx, &orders, queryBuilder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list receipt orders: %w", err)
	}
	if orders == nil {
		orders = make([]*model.ReceiptOrder, 0)
	}
	return orders, nil
}

func (r *receiptRepository) UpdateReceiptOrder(ctx context.Context, userID, id uuid.UUID, req *model.UpdateReceiptRequest) (*model.ReceiptOrder, error) {
	current, err := r.GetReceiptOrderByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	if req.VendorName != nil {
		current.VendorName = *req.VendorName
	}
	if req.Category != nil {
		current.Category = *req.Category
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.TotalPrice != nil {
		current.TotalPrice = *req.TotalPrice
	}
	if req.OrderDate != nil {
		current.OrderDate = *req.OrderDate
	}
	if req.FolderID != nil {
		current.FolderID = req.FolderID
	}

	query := `
		UPDATE receipt_orders
		SET vendor_name = $1, category = $2, description = $3, total_price = $4, order_date = $5::date, folder_id = $6
		WHERE id = $7 AND user_id = $8
	`
	res, err := r.db.ExecContext(ctx, query, current.VendorName, current.Category, current.Description, current.TotalPrice, current.OrderDate, current.FolderID, id, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to update receipt order: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return nil, ErrReceiptNotFound
	}

	return current, nil
}

func (r *receiptRepository) DeleteReceiptOrder(ctx context.Context, userID, id uuid.UUID) error {
	query := `DELETE FROM receipt_orders WHERE id = $1 AND user_id = $2`
	res, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete receipt order: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return ErrReceiptNotFound
	}
	return nil
}

func (r *receiptRepository) GetReceiptSummary(ctx context.Context, userID uuid.UUID, startDate, endDate, folderID string) (*model.ReceiptSummary, error) {
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
		SELECT COALESCE(category, 'Uncategorized') AS category, 
		       COALESCE(SUM(total_price), 0) AS total_spend, 
		       COUNT(id) AS count
		FROM receipt_orders
		WHERE user_id = $1
	`)

	args := []interface{}{userID}
	argIdx := 2

	if folderID != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND folder_id = $%d", argIdx))
		args = append(args, folderID)
		argIdx++
	}

	if startDate != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND order_date >= $%d::date", argIdx))
		args = append(args, startDate)
		argIdx++
	}

	if endDate != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND order_date <= $%d::date", argIdx))
		args = append(args, endDate)
		argIdx++
	}

	queryBuilder.WriteString(" GROUP BY category ORDER BY total_spend DESC")

	catSummaries := make([]model.CategoryExpenseSummary, 0)
	err := r.db.SelectContext(ctx, &catSummaries, queryBuilder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query receipt summary: %w", err)
	}
	if catSummaries == nil {
		catSummaries = make([]model.CategoryExpenseSummary, 0)
	}

	var totalSpend float64
	var totalCount int
	for _, c := range catSummaries {
		totalSpend += c.TotalSpend
		totalCount += c.Count
	}

	return &model.ReceiptSummary{
		TotalSpend: totalSpend,
		TotalCount: totalCount,
		Categories: catSummaries,
	}, nil
}
