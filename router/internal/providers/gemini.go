package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiProvider implements the Provider interface for Google Gemini
type GeminiProvider struct {
	config     ProviderConfig
	httpClient *http.Client
	pricing    map[string]ModelPricing
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(config ProviderConfig) *GeminiProvider {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	provider := &GeminiProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		pricing: map[string]ModelPricing{
			"gemini-1.5-pro": {
				InputPricePer1K:  0.0035,
				OutputPricePer1K: 0.0105,
				MaxTokens:        8192,
				ContextWindow:    2000000, // 2M tokens
			},
			"gemini-1.5-flash": {
				InputPricePer1K:  0.000075,
				OutputPricePer1K: 0.0003,
				MaxTokens:        8192,
				ContextWindow:    1000000, // 1M tokens
			},
			"gemini-pro": {
				InputPricePer1K:  0.0005,
				OutputPricePer1K: 0.0015,
				MaxTokens:        2048,
				ContextWindow:    30720, // ~30K tokens
			},
		},
	}

	// Override base URL in config
	provider.config.BaseURL = baseURL
	return provider
}

func (p *GeminiProvider) Name() string {
	return p.config.Name
}

func (p *GeminiProvider) Health(ctx context.Context) error {
	// Use the models list endpoint for health check
	url := fmt.Sprintf("%s/v1/models?key=%s", p.config.BaseURL, p.config.APIKey)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gemini health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func (p *GeminiProvider) Forward(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint string) error {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	// Parse and convert the request to Gemini format
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		return fmt.Errorf("failed to parse request JSON: %w", err)
	}

	// Convert to Gemini format
	geminiBody, model := p.convertToGeminiFormat(requestData)

	// Create target URL for Gemini API
	targetURL := fmt.Sprintf("%s/v1/models/%s:generateContent?key=%s", 
		p.config.BaseURL, model, p.config.APIKey)

	// Handle streaming
	if stream, ok := requestData["stream"].(bool); ok && stream {
		targetURL = fmt.Sprintf("%s/v1/models/%s:streamGenerateContent?key=%s", 
			p.config.BaseURL, model, p.config.APIKey)
	}

	// Create new request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(geminiBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Copy relevant headers from original request
	for name, values := range r.Header {
		if strings.ToLower(name) == "authorization" {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Make request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward to Gemini: %w", err)
	}
	defer resp.Body.Close()

	// Handle streaming response differently
	if strings.Contains(targetURL, "streamGenerateContent") {
		return p.handleStreamingResponse(w, resp, model)
	}

	// Handle regular response
	return p.handleRegularResponse(w, resp, model)
}

func (p *GeminiProvider) convertToGeminiFormat(requestData map[string]interface{}) ([]byte, string) {
	geminiRequest := map[string]interface{}{
		"contents": []map[string]interface{}{},
	}

	// Extract model
	model := p.config.DefaultModel
	if m, ok := requestData["model"].(string); ok {
		model = m
	}
	if model == "" {
		model = "gemini-pro"
	}

	// Convert messages to Gemini contents format
	if messages, ok := requestData["messages"].([]interface{}); ok {
		var parts []map[string]interface{}
		
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				role := "user"
				if r, ok := msgMap["role"].(string); ok {
					if r == "assistant" {
						role = "model"
					} else if r == "system" {
						// System messages need special handling in Gemini
						continue
					}
				}

				if content, ok := msgMap["content"].(string); ok {
					part := map[string]interface{}{
						"role": role,
						"parts": []map[string]interface{}{
							{"text": content},
						},
					}
					parts = append(parts, part)
				}
			}
		}
		
		geminiRequest["contents"] = parts
	}

	// Handle generation config
	generationConfig := make(map[string]interface{})
	
	if temp, ok := requestData["temperature"]; ok {
		generationConfig["temperature"] = temp
	}
	if topP, ok := requestData["top_p"]; ok {
		generationConfig["topP"] = topP
	}
	if maxTokens, ok := requestData["max_tokens"]; ok {
		generationConfig["maxOutputTokens"] = maxTokens
	}

	if len(generationConfig) > 0 {
		geminiRequest["generationConfig"] = generationConfig
	}

	body, _ := json.Marshal(geminiRequest)
	return body, model
}

func (p *GeminiProvider) handleRegularResponse(w http.ResponseWriter, resp *http.Response, model string) error {
	// Read Gemini response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Gemini response: %w", err)
	}

	// Convert to OpenAI format
	openaiResponse := p.convertFromGeminiFormat(responseBody, model)

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set content type
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	_, err = w.Write(openaiResponse)
	return err
}

func (p *GeminiProvider) handleStreamingResponse(w http.ResponseWriter, resp *http.Response, model string) error {
	// For streaming, we need to parse each chunk and convert format
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	// For now, read the entire response and convert
	// In production, you'd want to parse streaming chunks individually
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Gemini streaming response: %w", err)
	}

	// Convert and write as SSE format
	openaiResponse := p.convertFromGeminiFormat(responseBody, model)
	
	// Write as server-sent event
	fmt.Fprintf(w, "data: %s\n\n", string(openaiResponse))
	fmt.Fprintf(w, "data: [DONE]\n\n")

	return nil
}

func (p *GeminiProvider) convertFromGeminiFormat(geminiResponse []byte, model string) []byte {
	// Parse Gemini response
	var geminiData map[string]interface{}
	if err := json.Unmarshal(geminiResponse, &geminiData); err != nil {
		// If parsing fails, return error response
		errorResp := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Failed to parse Gemini response",
				"type":    "api_error",
			},
		}
		body, _ := json.Marshal(errorResp)
		return body
	}

	// Convert to OpenAI format
	openaiResponse := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": p.extractGeminiContent(geminiData),
				},
				"finish_reason": "stop",
			},
		},
	}

	// Add usage information if available
	if usageMetadata, ok := geminiData["usageMetadata"].(map[string]interface{}); ok {
		usage := map[string]interface{}{}
		if promptTokens, ok := usageMetadata["promptTokenCount"]; ok {
			usage["prompt_tokens"] = promptTokens
		}
		if completionTokens, ok := usageMetadata["candidatesTokenCount"]; ok {
			usage["completion_tokens"] = completionTokens
		}
		if totalTokens, ok := usageMetadata["totalTokenCount"]; ok {
			usage["total_tokens"] = totalTokens
		}
		if len(usage) > 0 {
			openaiResponse["usage"] = usage
		}
	}

	body, _ := json.Marshal(openaiResponse)
	return body
}

func (p *GeminiProvider) extractGeminiContent(geminiData map[string]interface{}) string {
	if candidates, ok := geminiData["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]interface{}); ok {
			if content, ok := candidate["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]interface{}); ok {
						if text, ok := part["text"].(string); ok {
							return text
						}
					}
				}
			}
		}
	}
	return ""
}

func (p *GeminiProvider) CalculateCost(inputTokens, outputTokens int) float64 {
	model := p.config.DefaultModel
	if model == "" {
		model = "gemini-pro" // fallback
	}

	pricing, exists := p.pricing[model]
	if !exists {
		// Use gemini-pro pricing as default
		pricing = p.pricing["gemini-pro"]
	}

	inputCost := float64(inputTokens) * pricing.InputPricePer1K / 1000.0
	outputCost := float64(outputTokens) * pricing.OutputPricePer1K / 1000.0

	return inputCost + outputCost
}

func (p *GeminiProvider) GetModelPricing() map[string]ModelPricing {
	return p.pricing
}
