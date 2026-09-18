package service

import (
	"context"
	"testing"
)

func TestPlayBillingServiceSandbox(t *testing.T) {
	svc, err := NewPlayBillingService(context.Background())
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}

	valid, err := svc.VerifySubscription(context.Background(), "com.scannerapp", "sub_pro_monthly", "mock_token_12345")
	if err != nil {
		t.Fatalf("expected sandbox token to succeed, got error: %v", err)
	}
	if !valid {
		t.Fatalf("expected sandbox token to be valid")
	}

	// Missing token
	_, err = svc.VerifySubscription(context.Background(), "com.scannerapp", "sub_pro_monthly", "")
	if err == nil {
		t.Fatalf("expected error on empty purchase token")
	}
}
