package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/pkg/response"
)

type OCRHandler struct {
	ocrService service.OCRService
	userRepo   postgres.UserRepository
}

func NewOCRHandler(ocrService service.OCRService, userRepo postgres.UserRepository) *OCRHandler {
	return &OCRHandler{
		ocrService: ocrService,
		userRepo:   userRepo,
	}
}

func (h *OCRHandler) Process(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tier := model.TierFree
	if h.userRepo != nil {
		if user, err := h.userRepo.GetUserByID(r.Context(), userID); err == nil && user != nil {
			tier = user.Tier
		}
	} else if tierStr, ok := middleware.GetUserTier(r.Context()); ok {
		tier = model.UserTier(tierStr)
	}

	var req model.OCRRequest

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse up to 32MB multipart
		if err := r.ParseMultipartForm(32 << 20); err == nil {
			file, _, err := r.FormFile("file")
			if err != nil {
				file, _, err = r.FormFile("image")
			}
			if err == nil {
				defer file.Close()
				data, err := io.ReadAll(file)
				if err == nil {
					req.ImageBase64 = base64.StdEncoding.EncodeToString(data)
				}
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if req.ImageBase64 == "" {
		response.Error(w, http.StatusBadRequest, "image file or image_base64 is required")
		return
	}

	res, err := h.ocrService.ProcessOCR(r.Context(), userID, tier, &req)
	if err != nil {
		if errors.Is(err, service.ErrOCRLimitExceeded) {
			response.Error(w, http.StatusTooManyRequests, err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.JSON(w, http.StatusOK, res)
}

func (h *OCRHandler) GetQuota(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tier := model.TierFree
	if h.userRepo != nil {
		if user, err := h.userRepo.GetUserByID(r.Context(), userID); err == nil && user != nil {
			tier = user.Tier
		}
	} else if tierStr, ok := middleware.GetUserTier(r.Context()); ok {
		tier = model.UserTier(tierStr)
	}

	remaining, err := h.ocrService.GetQuota(r.Context(), userID, tier)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"tier":           tier,
		"remaining_uses": remaining,
	})
}
