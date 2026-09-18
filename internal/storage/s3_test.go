package storage_test

import (
	"testing"

	"github.com/scannerapp/backend/internal/storage"
)

func TestNewS3Storage(t *testing.T) {
	// Verify S3Storage initializes with valid parameters
	s, err := storage.NewS3Storage("localhost:9000", "minioadmin", "minioadmin", false)
	if err != nil {
		t.Fatalf("unexpected error initializing S3Storage: %v", err)
	}

	if s == nil {
		t.Fatal("expected non-nil S3Storage instance")
	}
}
