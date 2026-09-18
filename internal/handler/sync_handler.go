package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/internal/storage"
	"github.com/scannerapp/backend/pkg/response"
)

type SyncHandler struct {
	syncService    service.SyncService
	storageService storage.StorageService
	s3Bucket       string
}

func NewSyncHandler(
	syncService service.SyncService,
	storageService storage.StorageService,
	s3Bucket string,
) *SyncHandler {
	return &SyncHandler{
		syncService:    syncService,
		storageService: storageService,
		s3Bucket:       s3Bucket,
	}
}

func (h *SyncHandler) Push(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req model.SyncPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.syncService.PushData(r.Context(), userID, &req); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "synchronized"})
}

func (h *SyncHandler) Pull(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var since int64 = 0
	if sinceQuery := r.URL.Query().Get("since"); sinceQuery != "" {
		if val, err := strconv.ParseInt(sinceQuery, 10, 64); err == nil {
			since = val
		}
	}

	data, err := h.syncService.PullData(r.Context(), userID, since)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.JSON(w, http.StatusOK, data)
}

func (h *SyncHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]
	if docID == "" {
		response.Error(w, http.StatusBadRequest, "missing document id")
		return
	}

	// Parse up to 32MB multipart
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "file part is required")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/pdf"
	}

	// Unique S3 storage key: documents/{userID}/{documentID}/{filename}
	storageKey := fmt.Sprintf("documents/%s/%s/%s", userID.String(), docID, header.Filename)

	// Stream multipart file directly to MinIO
	_, err = h.storageService.Upload(r.Context(), h.s3Bucket, storageKey, file, header.Size, contentType)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to upload file to storage: %v", err))
		return
	}

	// Update documents table in PostgreSQL with storage key
	err = h.syncService.UpdateDocumentFile(r.Context(), userID, docID, storageKey)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to record file metadata: %v", err))
		return
	}

	// Generate download URL
	downloadURL, _ := h.storageService.GetDownloadURL(r.Context(), h.s3Bucket, storageKey)

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"document_id":  docID,
		"filename":     header.Filename,
		"size":         header.Size,
		"storage_key":  storageKey,
		"download_url": downloadURL,
		"status":       "uploaded",
	})
}

func (h *SyncHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]
	if docID == "" {
		response.Error(w, http.StatusBadRequest, "missing document id")
		return
	}

	doc, err := h.syncService.GetDocument(r.Context(), userID, docID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "document not found")
		return
	}

	var downloadURL string
	if doc.PdfCloudURL != nil && *doc.PdfCloudURL != "" {
		url, err := h.storageService.GetDownloadURL(r.Context(), h.s3Bucket, *doc.PdfCloudURL)
		if err == nil {
			downloadURL = url
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"document":     doc,
		"download_url": downloadURL,
	})
}
