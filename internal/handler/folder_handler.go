package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/pkg/response"
)

type FolderHandler struct {
	folderService service.FolderService
}

func NewFolderHandler(folderService service.FolderService) *FolderHandler {
	return &FolderHandler{folderService: folderService}
}

// Create handles POST /api/v1/folders
func (h *FolderHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req model.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "Folder name is required")
		return
	}

	folder, err := h.folderService.CreateFolder(r.Context(), userID, req.Name)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create folder: "+err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, folder)
}

// List handles GET /api/v1/folders
func (h *FolderHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	folders, err := h.folderService.ListFolders(r.Context(), userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list folders: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, folders)
}

// GetByID handles GET /api/v1/folders/{id}
func (h *FolderHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		response.Error(w, http.StatusBadRequest, "Folder ID is required")
		return
	}

	folder, err := h.folderService.GetFolder(r.Context(), userID, id)
	if err != nil {
		if err == postgres.ErrFolderNotFound {
			response.Error(w, http.StatusNotFound, "Folder not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get folder: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, folder)
}

// Delete handles DELETE /api/v1/folders/{id}
func (h *FolderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		response.Error(w, http.StatusBadRequest, "Folder ID is required")
		return
	}

	if err := h.folderService.DeleteFolder(r.Context(), userID, id); err != nil {
		if err == postgres.ErrFolderNotFound {
			response.Error(w, http.StatusNotFound, "Folder not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to delete folder: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"message": "Folder deleted successfully"})
}