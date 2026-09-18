package service

import (
	"context"
	"testing"
)

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean raw json",
			input:    `{"vendor_name": "Home Depot", "total_price": 45.00}`,
			expected: `{"vendor_name": "Home Depot", "total_price": 45.00}`,
		},
		{
			name:     "wrapped with markdown code block",
			input:    "```json\n{\"vendor_name\": \"Home Depot\", \"total_price\": 45.00}\n```",
			expected: `{"vendor_name": "Home Depot", "total_price": 45.00}`,
		},
		{
			name:     "wrapped with generic markdown block",
			input:    "```\n{\"vendor_name\": \"Starbucks\", \"total_price\": 6.50}\n```",
			expected: `{"vendor_name": "Starbucks", "total_price": 6.50}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanJSONResponse(tt.input)
			if got != tt.expected {
				t.Errorf("cleanJSONResponse() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGeminiServiceFallback(t *testing.T) {
	service := NewGeminiService("", "gemini-3.6-flash")
	extracted, err := service.ExtractReceiptData(context.Background(), []byte("dummy-image-bytes"), "image/jpeg")
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}

	if extracted.VendorName == "" {
		t.Errorf("expected non-empty vendor name in fallback mode")
	}
	if extracted.TotalPrice <= 0 {
		t.Errorf("expected positive total price in fallback mode, got: %f", extracted.TotalPrice)
	}
}
