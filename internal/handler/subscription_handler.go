package handler

import (
	"encoding/json"
	"net/http"

	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/pkg/response"
)

type SubscriptionHandler struct {
	billingService service.PlayBillingService
	userRepo       postgres.UserRepository
}

func NewSubscriptionHandler(billingService service.PlayBillingService, userRepo postgres.UserRepository) *SubscriptionHandler {
	return &SubscriptionHandler{
		billingService: billingService,
		userRepo:       userRepo,
	}
}

type VerifySubscriptionRequest struct {
	PurchaseToken  string `json:"purchase_token"`
	SubscriptionID string `json:"subscription_id"`
	PackageName    string `json:"package_name,omitempty"`
}

func (h *SubscriptionHandler) Verify(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req VerifySubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PurchaseToken == "" || req.SubscriptionID == "" {
		response.Error(w, http.StatusBadRequest, "purchase_token and subscription_id are required")
		return
	}

	valid, err := h.billingService.VerifySubscription(r.Context(), req.PackageName, req.SubscriptionID, req.PurchaseToken)
	if err != nil || !valid {
		msg := "subscription verification failed"
		if err != nil {
			msg = err.Error()
		}
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	// Update user's tier to premium in PostgreSQL
	if err := h.userRepo.UpdateUserTier(r.Context(), userID, model.TierPremium); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to update user tier: "+err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"tier":    string(model.TierPremium),
		"message": "Subscription verified successfully. Upgraded to premium!",
	})
}
