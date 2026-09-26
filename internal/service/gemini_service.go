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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/scannerapp/backend/internal/model"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
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
		modelName = "gemini-1.5-flash"
	}

	return &geminiService{
		apiKey: apiKey,
		model:  modelName,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

const receiptPrompt = `Analyze this receipt image thoroughly and extract the exact expense information into structured JSON.
Return ONLY a valid JSON object with the following fields:
{
  "vendor_name": "Store, contractor, or business name (e.g. Home Depot, Starbucks, Acme Construction)",
  "order_date": "YYYY-MM-DD",
  "category": "Broad classification (e.g. Construction, Dining, Hardware, Office Supplies, Travel, Utilities, Retail)",
  "description": "Combine all items into a single descriptive string (e.g. 100 Bags of Portland Cement & Materials)",
  "total_price": 0.00
}
If order_date is missing or unreadable, use today's date in YYYY-MM-DD format.
total_price must be a decimal number without currency symbols.
Return ONLY raw JSON. Do not include markdown formatting, backticks, or code fences.`

// ReceiptData represents the structured JSON output expected by Android client
type ReceiptData struct {
	VendorName  string  `json:"vendor_name"`
	OrderDate   string  `json:"order_date"`
	Category    string  `json:"category"`
	Description string  `json:"description"` // Combine all items into a single descriptive string
	TotalPrice  float64 `json:"total_price"`
}

func (r *ReceiptData) UnmarshalJSON(data []byte) error {
	type Alias ReceiptData
	aux := struct {
		RawPrice interface{} `json:"total_price"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.TotalPrice = parseFlexPrice(aux.RawPrice)
	return nil
}

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

	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("receipt image bytes are empty")
	}

	// Clean and normalize MIME type
	cleanMime := strings.Split(mimeType, ";")[0]
	cleanMime = strings.TrimSpace(cleanMime)
	if cleanMime == "" {
		cleanMime = "image/jpeg"
	}

	// Determine candidate models
	candidateModels := s.buildCandidateModels(ctx)

	var lastErr error
	var rawText string

	// Attempt 1: Try official Google Generative AI SDK
	rawText, lastErr = s.extractWithSDK(ctx, imageBytes, cleanMime, candidateModels)

	// Attempt 2: If SDK fails, fallback to direct REST v1beta calls
	if rawText == "" {
		log.Printf("Notice: Generative AI SDK did not return text (%v). Trying REST fallback...", lastErr)
		rawText, lastErr = s.extractWithREST(ctx, imageBytes, cleanMime, candidateModels)
	}

	// If all attempts failed, return the actual error so the client knows and can retry
	if rawText == "" {
		log.Printf("ERROR: All Gemini extraction attempts failed: %v", lastErr)
		return nil, fmt.Errorf("AI receipt extraction failed (please try again): %w", lastErr)
	}

	// Clean and strictly parse JSON
	cleanJSON := cleanJSONResponse(rawText)
	var parsed ReceiptData
	if err := json.Unmarshal([]byte(cleanJSON), &parsed); err != nil {
		log.Printf("ERROR: Failed to unmarshal Gemini receipt extraction JSON: %v. Raw AI text:\n%s", err, rawText)
		return nil, fmt.Errorf("failed to parse receipt JSON from AI response: %w (raw response: %s)", err, rawText)
	}

	vendor := strings.TrimSpace(parsed.VendorName)
	if vendor == "" {
		vendor = "Unknown Vendor"
	}

	category := strings.TrimSpace(parsed.Category)
	if category == "" {
		category = "General"
	}

	orderDate := strings.TrimSpace(parsed.OrderDate)
	if orderDate == "" {
		orderDate = time.Now().Format("2006-01-02")
	}

	totalPrice := parsed.TotalPrice

	return &model.ReceiptExtractedData{
		VendorName:  vendor,
		OrderDate:   orderDate,
		Category:    category,
		Description: strings.TrimSpace(parsed.Description),
		TotalPrice:  totalPrice,
		Confidence:  0.98,
	}, nil
}

// extractWithSDK uses the official github.com/google/generative-ai-go/genai SDK
func (s *geminiService) extractWithSDK(ctx context.Context, imageBytes []byte, mimeType string, models []string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create genai client: %w", err)
	}
	defer client.Close()

	var lastErr error
	for _, modelName := range models {
		cleanName := strings.TrimPrefix(modelName, "models/")
		model := client.GenerativeModel(cleanName)
		model.SetTemperature(0.1)
		model.ResponseMIMEType = "application/json"

		// Supported image formats for genai.ImageData: "image/png", "image/jpeg", etc.
		format := strings.TrimPrefix(mimeType, "image/")
		if format == "jpg" {
			format = "jpeg"
		}

		resp, err := model.GenerateContent(ctx,
			genai.Text(receiptPrompt),
			genai.ImageData(format, imageBytes),
		)
		if err != nil {
			lastErr = err
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "404") || strings.Contains(lower, "not found") {
				log.Printf("Notice: genai SDK model '%s' returned 404. Trying next model...", cleanName)
				continue
			}
			if strings.Contains(lower, "429") || strings.Contains(lower, "quota") {
				log.Printf("Notice: genai SDK model '%s' returned 429 quota limit. Trying next model...", cleanName)
				continue
			}
			log.Printf("Notice: genai SDK model '%s' returned error: %v. Trying next model...", cleanName, err)
			continue
		}

		if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			lastErr = fmt.Errorf("model %s returned no candidates", cleanName)
			continue
		}

		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok && string(txt) != "" {
				return string(txt), nil
			}
		}
	}

	return "", lastErr
}

// extractWithREST provides a direct HTTP REST fallback using v1beta endpoints
func (s *geminiService) extractWithREST(ctx context.Context, imageBytes []byte, mimeType string, models []string) (string, error) {
	encodedImage := base64.StdEncoding.EncodeToString(imageBytes)

	type restPart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"inline_data,omitempty"`
	}

	type restReq struct {
		Contents []struct {
			Parts []restPart `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			Temperature      float64 `json:"temperature"`
			ResponseMimeType string  `json:"response_mime_type,omitempty"`
		} `json:"generationConfig"`
	}

	reqBody := restReq{
		Contents: []struct {
			Parts []restPart `json:"parts"`
		}{
			{
				Parts: []restPart{
					{Text: receiptPrompt},
					{
						InlineData: &struct {
							MimeType string `json:"mime_type"`
							Data     string `json:"data"`
						}{
							MimeType: mimeType,
							Data:     encodedImage,
						},
					},
				},
			},
		},
	}
	reqBody.GenerationConfig.Temperature = 0.1
	reqBody.GenerationConfig.ResponseMimeType = "application/json"

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal rest request: %w", err)
	}

	var lastErr error
	for _, m := range models {
		cleanName := strings.TrimPrefix(m, "models/")
		// Use strictly v1beta for model flexibility
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", cleanName, s.apiKey)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		var gResp struct {
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
			} `json:"error,omitempty"`
		}

		if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
			lastErr = fmt.Errorf("failed to parse REST response for %s: %w", cleanName, err)
			continue
		}

		if gResp.Error != nil {
			log.Printf("Notice: REST model %s returned code %d: %s", cleanName, gResp.Error.Code, gResp.Error.Message)
			lastErr = fmt.Errorf("gemini rest error (%d): %s", gResp.Error.Code, gResp.Error.Message)
			if gResp.Error.Code == 404 || strings.Contains(strings.ToLower(gResp.Error.Message), "not found") {
				continue
			}
			if gResp.Error.Code == 429 {
				log.Printf("Notice: REST model %s returned 429 quota limit. Trying next model...", cleanName)
				continue
			}
			continue
		}

		if len(gResp.Candidates) > 0 && len(gResp.Candidates[0].Content.Parts) > 0 {
			return gResp.Candidates[0].Content.Parts[0].Text, nil
		}
	}

	return "", lastErr
}

// buildCandidateModels returns an ordered, deduplicated slice of models to try
func (s *geminiService) buildCandidateModels(ctx context.Context) []string {
	var candidates []string
	add := func(m string) {
		clean := strings.TrimPrefix(strings.TrimSpace(m), "models/")
		if clean == "" {
			return
		}
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "tts") || strings.Contains(lower, "audio") || strings.Contains(lower, "embed") || strings.Contains(lower, "search") || strings.Contains(lower, "imagen") {
			return
		}
		for _, existing := range candidates {
			if existing == clean {
				return
			}
		}
		candidates = append(candidates, clean)
	}

	// 1. High-quota stable multimodal vision models first!
	stableVisionModels := []string{
		"gemini-1.5-flash",
		"gemini-2.0-flash",
		"gemini-1.5-flash-latest",
		"gemini-1.5-flash-8b",
		"gemini-1.5-pro",
		"gemini-2.5-flash",
		"gemini-pro",
	}
	for _, m := range stableVisionModels {
		add(m)
	}

	// 2. Configured model
	if s.model != "" {
		add(s.model)
	}

	// 3. Dynamically discovered vision models from ListModels
	discovered := s.getAvailableModels(ctx)
	for _, m := range discovered {
		add(m)
	}

	return candidates
}

func (s *geminiService) getAvailableModels(ctx context.Context) []string {
	if s.apiKey == "" {
		return nil
	}

	s.mu.RLock()
	if len(s.discoveredModels) > 0 && time.Since(s.discoveredAt) < 30*time.Minute {
		cached := make([]string, len(s.discoveredModels))
		copy(cached, s.discoveredModels)
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	// Try querying ListModels via SDK first
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err == nil {
		defer client.Close()
		iter := client.ListModels(ctx)
		var flashModels []string
		var otherModels []string
		for {
			m, iterErr := iter.Next()
			if iterErr == iterator.Done || iterErr != nil {
				break
			}
			clean := strings.TrimPrefix(m.Name, "models/")
			lower := strings.ToLower(clean)
			// Exclude non-vision/multimodal models (TTS, audio-only, embeddings)
			if strings.Contains(lower, "tts") || strings.Contains(lower, "audio") || strings.Contains(lower, "embed") || strings.Contains(lower, "search") || strings.Contains(lower, "imagen") {
				continue
			}
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

			if strings.Contains(lower, "flash") {
				flashModels = append(flashModels, clean)
			} else if strings.HasPrefix(lower, "gemini") {
				otherModels = append(otherModels, clean)
			}
		}

		combined := append(flashModels, otherModels...)
		if len(combined) > 0 {
			log.Printf("ListModels SDK discovered %d Gemini models: %v", len(combined), combined)
			s.mu.Lock()
			s.discoveredModels = combined
			s.discoveredAt = time.Now()
			s.mu.Unlock()
			return combined
		}
	}

	return nil
}

// parseFlexPrice parses numeric or formatted strings into a clean float64
func parseFlexPrice(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		cleaned := strings.TrimSpace(v)
		cleaned = strings.ReplaceAll(cleaned, "$", "")
		cleaned = strings.ReplaceAll(cleaned, "€", "")
		cleaned = strings.ReplaceAll(cleaned, "£", "")
		cleaned = strings.ReplaceAll(cleaned, "₹", "")
		cleaned = strings.ReplaceAll(cleaned, ",", "")
		cleaned = strings.TrimSpace(cleaned)
		if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
			return f
		}
	}
	return 0.0
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
