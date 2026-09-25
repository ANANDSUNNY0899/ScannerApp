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
		{
			name:     "preamble and postamble text",
			input:    "Here is your json:\n```json\n{\"vendor_name\": \"Walmart\", \"total_price\": \"$88.42\"}\n```\nHope that helps!",
			expected: `{"vendor_name": "Walmart", "total_price": "$88.42"}`,
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

func TestParseFlexPrice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{"float64", 45.50, 45.50},
		{"float32", float32(12.25), 12.25},
		{"int", 100, 100.0},
		{"string plain", "75.80", 75.80},
		{"string with dollar", "$124.99", 124.99},
		{"string with comma", "$1,450.50", 1450.50},
		{"string with spaces", "  50.00  ", 50.00},
		{"invalid string", "unknown", 0.0},
		{"nil value", nil, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFlexPrice(tt.input)
			if got != tt.expected {
				t.Errorf("parseFlexPrice(%v) = %f, want %f", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGeminiServiceFallback(t *testing.T) {
	service := NewGeminiService("", "gemini-1.5-flash")
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
