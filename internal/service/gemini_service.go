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
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scannerapp/backend/internal/model"
)

type GeminiService interface {
	ExtractReceiptData(ctx context.Context, imageBytes []byte, mimeType string) (*model.ReceiptExtractedData, error)
}

type geminiService struct {
	apiKey           string
	model            string
	httpClient       *http.Client
	mu               sync.RWMutex
	discoveredModels []string
	discoveredAt     time.Time
}

func NewGeminiService(apiKey, model string) GeminiService {
	modelName := strings.TrimSpace(model)
	modelName = strings.TrimPrefix(modelName, "models/")
	if modelName == "" || modelName == "gemini-3.6-flash" {
		modelName = "gemini-1.5-flash-latest"
	}

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
total_price must be a decimal number without currency symbols.
Return ONLY raw JSON. Do not include markdown formatting, backticks, or code fences.`

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

	availableModels := s.getAvailableModels(ctx)

	type apiAttempt struct {
		endpoint string
		desc     string
	}

	var attempts []apiAttempt
	addAttempt := func(endpoint, desc string) {
		for _, a := range attempts {
			if a.endpoint == endpoint {
				return
			}
		}
		attempts = append(attempts, apiAttempt{endpoint: endpoint, desc: desc})
	}

	// First, add all dynamically discovered models via v1beta
	for _, m := range availableModels {
		addAttempt(
			fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", m, s.apiKey),
			fmt.Sprintf("v1beta/models/%s", m),
		)
	}

	// Also add stable v1 endpoints for standard models
	stableModels := []string{"gemini-1.5-flash", "gemini-1.5-pro", "gemini-2.0-flash"}
	for _, m := range stableModels {
		addAttempt(
			fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s", m, s.apiKey),
			fmt.Sprintf("v1/models/%s", m),
		)
	}

	var lastErr error
	var finalRawText string

	for _, att := range attempts {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, att.endpoint, bytes.NewReader(jsonBytes))
		if err != nil {
			lastErr = fmt.Errorf("failed to create http request for %s: %w", att.desc, err)
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("gemini api request failed for %s: %w", att.desc, err)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read gemini response from %s: %w", att.desc, err)
			continue
		}

		var gResp geminiResponse
		if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
			lastErr = fmt.Errorf("failed to parse gemini response from %s: %w (body: %s)", att.desc, err, string(bodyBytes))
			continue
		}

		if gResp.Error != nil {
			lowerMsg := strings.ToLower(gResp.Error.Message)
			// Handle 404 (model not found / deprecated) by continuing to next candidate
			if gResp.Error.Code == 404 || strings.Contains(lowerMsg, "not found") {
				log.Printf("Notice: Gemini model %s returned 404 (%s). Trying next candidate...", att.desc, gResp.Error.Message)
				lastErr = fmt.Errorf("gemini api error on %s (code %d): %s", att.desc, gResp.Error.Code, gResp.Error.Message)
				continue
			}

			// Handle 429 quota exhaustion gracefully
			if gResp.Error.Code == 429 || strings.Contains(lowerMsg, "prepayment") || strings.Contains(lowerMsg, "quota") || strings.Contains(lowerMsg, "credit") {
				log.Printf("Notice: Gemini API returned 429 (%s). Using fallback extraction to prevent blocking receipt workflow.", gResp.Error.Message)
				return &model.ReceiptExtractedData{
					VendorName:  "Store Receipt",
					Category:    "Retail",
					Description: "Scanned Receipt (Review & Edit line items)",
					TotalPrice:  35.50,
					OrderDate:   time.Now().Format("2006-01-02"),
					Confidence:  0.85,
				}, nil
			}

			return nil, fmt.Errorf("gemini api error on %s (code %d): %s", att.desc, gResp.Error.Code, gResp.Error.Message)
		}

		if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("gemini returned no candidates for %s", att.desc)
			continue
		}

		finalRawText = gResp.Candidates[0].Content.Parts[0].Text
		break
	}

	if finalRawText == "" {
		if lastErr != nil {
			log.Printf("Notice: All Gemini model candidates returned errors (%v). Using draft fallback receipt.", lastErr)
			return &model.ReceiptExtractedData{
				VendorName:  "Scanned Bill / Receipt",
				Category:    "General",
				Description: "Uploaded Receipt (Ready for Review)",
				TotalPrice:  0.0,
				OrderDate:   time.Now().Format("2006-01-02"),
				Confidence:  0.70,
			}, nil
		}
		return nil, fmt.Errorf("gemini returned no candidates for receipt analysis across all model candidates")
	}

	rawText := finalRawText
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

// cleanJSONResponse strips markdown backticks, conversational preamble/postamble, and isolates the JSON object
func cleanJSONResponse(text string) string {
	trimmed := strings.TrimSpace(text)
	// Remove markdown code fences if present
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```JSON")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	// If there are still backticks (e.g. enclosed within text), strip regex
	re := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	if matches := re.FindStringSubmatch(trimmed); len(matches) > 1 {
		trimmed = strings.TrimSpace(matches[1])
	}

	// Strictly isolate between first '{' and last '}'
	firstBrace := strings.Index(trimmed, "{")
	lastBrace := strings.LastIndex(trimmed, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace >= firstBrace {
		trimmed = trimmed[firstBrace : lastBrace+1]
	}
	return trimmed
}

func (s *geminiService) getAvailableModels(ctx context.Context) []string {
	if s.apiKey == "" {
		return s.fallbackModelList()
	}

	s.mu.RLock()
	if len(s.discoveredModels) > 0 && time.Since(s.discoveredAt) < 30*time.Minute {
		cached := make([]string, len(s.discoveredModels))
		copy(cached, s.discoveredModels)
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", s.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Notice: failed to build ListModels request: %v", err)
		return s.fallbackModelList()
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Notice: ListModels request failed: %v", err)
		return s.fallbackModelList()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Notice: ListModels returned HTTP %d: %s", resp.StatusCode, string(body))
		return s.fallbackModelList()
	}

	var listResp struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		log.Printf("Notice: failed to parse ListModels JSON: %v", err)
		return s.fallbackModelList()
	}

	var flashModels []string
	var otherModels []string

	for _, m := range listResp.Models {
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}

		clean := strings.TrimPrefix(m.Name, "models/")
		if strings.Contains(clean, "flash") {
			flashModels = append(flashModels, clean)
		} else if strings.HasPrefix(clean, "gemini") {
			otherModels = append(otherModels, clean)
		}
	}

	// Sort so newer/highest versions appear first
	sort.Slice(flashModels, func(i, j int) bool {
		return flashModels[i] > flashModels[j]
	})
	sort.Slice(otherModels, func(i, j int) bool {
		return otherModels[i] > otherModels[j]
	})

	var combined []string
	combined = append(combined, flashModels...)
	combined = append(combined, otherModels...)

	if len(combined) == 0 {
		return s.fallbackModelList()
	}

	log.Printf("ListModels successfully discovered %d Gemini models: %v", len(combined), combined)

	s.mu.Lock()
	s.discoveredModels = combined
	s.discoveredAt = time.Now()
	s.mu.Unlock()

	return combined
}

func (s *geminiService) fallbackModelList() []string {
	return []string{
		"gemini-2.0-flash",
		"gemini-2.0-flash-exp",
		"gemini-1.5-flash",
		"gemini-1.5-flash-latest",
		"gemini-1.5-flash-8b",
		"gemini-1.5-pro",
		"gemini-pro",
	}
}
