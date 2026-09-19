package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/pkg/response"
)

type ReceiptHandler struct {
	receiptService service.ReceiptService
}

func NewReceiptHandler(receiptService service.ReceiptService) *ReceiptHandler {
	return &ReceiptHandler{receiptService: receiptService}
}

// Extract processes an uploaded receipt image via Google Gemini Multimodal and persists the record.
func (h *ReceiptHandler) Extract(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("PANIC recovered in ReceiptHandler.Extract: %v", rec)
			response.Error(w, http.StatusInternalServerError, fmt.Sprintf("Internal server error during receipt extraction: %v", rec))
		}
	}()

	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 20 MB max file upload
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid multipart form: "+err.Error())
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		file, fileHeader, err = r.FormFile("receipt_image")
	}
	if err != nil || file == nil || fileHeader == nil {
		response.Error(w, http.StatusBadRequest, "Missing or invalid 'file' form field")
		return
	}
	defer file.Close()

	mimeType := fileHeader.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	order, err := h.receiptService.ExtractAndSaveReceipt(
		r.Context(),
		userID,
		file,
		fileHeader.Size,
		fileHeader.Filename,
		mimeType,
	)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to extract and save receipt: "+err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, order)
}

// List returns a paginated and filtered list of the user's receipts.
func (h *ReceiptHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := model.ReceiptFilter{
		Vendor:    q.Get("vendor"),
		Category:  q.Get("category"),
		StartDate: q.Get("start_date"),
		EndDate:   q.Get("end_date"),
		Limit:     limit,
		Offset:    offset,
	}

	orders, err := h.receiptService.ListReceipts(r.Context(), userID, filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list receipts: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, orders)
}

// GetByID returns details of a specific receipt owned by the user.
func (h *ReceiptHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid receipt ID")
		return
	}

	order, err := h.receiptService.GetReceipt(r.Context(), userID, id)
	if err != nil {
		if err == postgres.ErrReceiptNotFound {
			response.Error(w, http.StatusNotFound, "Receipt order not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to retrieve receipt: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, order)
}

// Update allows user to edit fields of a receipt order.
func (h *ReceiptHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid receipt ID")
		return
	}

	var req model.UpdateReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	updated, err := h.receiptService.UpdateReceipt(r.Context(), userID, id, &req)
	if err != nil {
		if err == postgres.ErrReceiptNotFound {
			response.Error(w, http.StatusNotFound, "Receipt order not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to update receipt: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, updated)
}

// Delete removes a receipt order owned by the user.
func (h *ReceiptHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid receipt ID")
		return
	}

	if err := h.receiptService.DeleteReceipt(r.Context(), userID, id); err != nil {
		if err == postgres.ErrReceiptNotFound {
			response.Error(w, http.StatusNotFound, "Receipt order not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to delete receipt: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"message": "Receipt deleted successfully"})
}

// Summary returns category and total spend aggregations.
func (h *ReceiptHandler) Summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	q := r.URL.Query()
	startDate := q.Get("start_date")
	endDate := q.Get("end_date")

	summary, err := h.receiptService.GetSummary(r.Context(), userID, startDate, endDate)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to compute receipt summary: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, summary)
}

// ExportCSV streams an Excel-compatible CSV ledger of all filtered receipts.
func (h *ReceiptHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	q := r.URL.Query()
	filter := model.ReceiptFilter{
		Vendor:    q.Get("vendor"),
		Category:  q.Get("category"),
		StartDate: q.Get("start_date"),
		EndDate:   q.Get("end_date"),
	}

	csvData, err := h.receiptService.GenerateLedgerCSV(r.Context(), userID, filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to generate CSV export: "+err.Error())
		return
	}

	filename := fmt.Sprintf("receipt_ledger_%s.csv", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvData)
}
