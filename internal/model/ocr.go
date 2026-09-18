package model

type OCRRequest struct {
	ImageBase64 string `json:"image_base64"`
	Language    string `json:"language"`
}

type OCRResponse struct {
	ExtractedText string  `json:"extracted_text"`
	Confidence    float64 `json:"confidence"`
	RemainingUses int     `json:"remaining_uses"`
}
