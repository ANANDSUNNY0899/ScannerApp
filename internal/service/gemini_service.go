package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/scannerapp/backend/internal/model"
)

type GeminiService interface {
	ExtractReceiptData(ctx context.Context, imageBytes []byte, mimeType string) (*model.ReceiptExtractedData, error)
}

type geminiService struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewGeminiService(apiKey, modelName string) GeminiService {
	// Prioritize GEMINI_MODEL environment variable, fallback to gemini-2.5-flash
	if envModel := os.Getenv("GEMINI_MODEL"); envModel != "" {
		modelName = envModel
	} else if modelName == "" || modelName == "gemini-1.5-flash" {
		modelName = "gemini-2.5-flash"
	}
	modelName = strings.TrimPrefix(modelName, "models/")

	return &geminiService{
		apiKey: apiKey,
		model:  modelName,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig *geminiGenCfg   `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiGenCfg struct {
	Temperature      float64 `json:"temperature"`
	ResponseMimeType string  `json:"response_mime_type,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

const receiptPrompt = `Analyze this receipt image thoroughly and extract the exact expense information into structured JSON.
Return ONLY a valid JSON object with the following fields:
{
  "vendor_name": "Store, contractor, or business name (e.g. Home Depot, Starbucks, Acme Construction)",
  "category": "Broad classification (e.g. Construction, Dining, Hardware, Office Supplies, Travel, Utilities, Retail)",
  "description": "Short summary of the purchased items or services (e.g. 100 Bags of Cement, Dinner meeting)",
  "total_price": 0.00,
  "order_date": "YYYY-MM-DD"
}
If order_date is missing or unreadable, use today's date in YYYY-MM-DD format.
total_price must be a decimal number without currency symbols.`

func (s *geminiService) ExtractReceiptData(ctx context.Context, imageBytes []byte, mimeType string) (*model.ReceiptExtractedData, error) {
	// Fallback mock mode when GEMINI_API_KEY is not configured
	if s.apiKey == "" {
		log.Println("Notice: GEMINI_API_KEY is not set. GeminiService running in simulated fallback mode.")
		return &model.ReceiptExtractedData{
			VendorName:  "Home Depot & Supplies",
			Category:    "Hardware",
			Description: "100 Bags of Portland Cement & Materials",
			TotalPrice:  450.50,
			OrderDate:   time.Now().Format("2006-01-02"),
			Confidence:  0.95,
		}, nil
	}

	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	encodedImage := base64.StdEncoding.EncodeToString(imageBytes)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: receiptPrompt},
					{
						InlineData: &geminiInlineData{
							MimeType: mimeType,
							Data:     encodedImage,
						},
					},
				},
			},
		},
		GenerationConfig: &geminiGenCfg{
			Temperature:      0.1,
			ResponseMimeType: "application/json",
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	modelName := strings.TrimPrefix(s.model, "models/")
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", modelName, s.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request for gemini: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini api request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemini response: %w", err)
	}

	var gResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
		return nil, fmt.Errorf("failed to parse gemini response: %w (body: %s)", err, string(bodyBytes))
	}

	if gResp.Error != nil {
		return nil, fmt.Errorf("gemini api error (code %d): %s", gResp.Error.Code, gResp.Error.Message)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no candidates for receipt analysis")
	}

	rawText := gResp.Candidates[0].Content.Parts[0].Text
	cleanJSON := cleanJSONResponse(rawText)

	var extracted model.ReceiptExtractedData
	if err := json.Unmarshal([]byte(cleanJSON), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse extracted receipt json: %w (raw: %s)", err, rawText)
	}

	if extracted.OrderDate == "" {
		extracted.OrderDate = time.Now().Format("2006-01-02")
	}
	extracted.Confidence = 0.98

	return &extracted, nil
}

// cleanJSONResponse strips markdown backticks and wraps if needed
func cleanJSONResponse(text string) string {
	trimmed := strings.TrimSpace(text)
	// Strip ```json ... ``` or ``` ... ```
	re := regexp.MustCompile("(?s)^```(?:json)?\\s*(.*)\\s*```$")
	if matches := re.FindStringSubmatch(trimmed); len(matches) > 1 {
		trimmed = strings.TrimSpace(matches[1])
	}
	return trimmed
}
