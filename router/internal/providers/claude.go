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

	"github.com/sirupsen/logrus"
)

// ClaudeProvider implements the Provider interface for Anthropic Claude
type ClaudeProvider struct {
	config     ProviderConfig
	httpClient *http.Client
	pricing    map[string]ModelPricing
}

// NewClaudeProvider creates a new Claude provider
func NewClaudeProvider(config ProviderConfig) *ClaudeProvider {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	provider := &ClaudeProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		pricing: map[string]ModelPricing{
			"claude-3-5-sonnet-20241022": {
				InputPricePer1K:  0.003,
				OutputPricePer1K: 0.015,
				MaxTokens:        8192,
				ContextWindow:    200000,
			},
			"claude-3-opus-20240229": {
				InputPricePer1K:  0.015,
				OutputPricePer1K: 0.075,
				MaxTokens:        4096,
				ContextWindow:    200000,
			},
			"claude-3-sonnet-20240229": {
				InputPricePer1K:  0.003,
				OutputPricePer1K: 0.015,
				MaxTokens:        4096,
				ContextWindow:    200000,
			},
			"claude-3-haiku-20240307": {
				InputPricePer1K:  0.00025,
				OutputPricePer1K: 0.00125,
				MaxTokens:        4096,
				ContextWindow:    200000,
			},
		},
	}

	// Override base URL in config
	provider.config.BaseURL = baseURL
	return provider
}

func (p *ClaudeProvider) Name() string {
	return p.config.Name
}

func (p *ClaudeProvider) Health(ctx context.Context) error {
	// Claude doesn't have a simple health endpoint, so we'll make a minimal request
	reqBody := map[string]interface{}{
		"model":      p.config.DefaultModel,
		"max_tokens": 1,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept both 200 and 429 (rate limit) as healthy responses
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("Claude health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func (p *ClaudeProvider) Forward(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint string) error {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	// Parse and potentially modify the request for Claude's format
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		logrus.Warnf("Failed to parse request JSON, forwarding as-is: %v", err)
	} else {
		// Convert OpenAI format to Claude format if needed
		body = p.convertToClaudeFormat(requestData)
	}

	// Create target URL - Claude uses /v1/messages for chat completions
	targetURL := p.config.BaseURL + "/v1/messages"
	if strings.Contains(endpoint, "completions") && !strings.Contains(endpoint, "chat") {
		// For non-chat completions, we'll need to convert format
		targetURL = p.config.BaseURL + "/v1/messages"
	}

	// Create new request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set Claude-specific headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Copy relevant headers from original request (excluding auth)
	for name, values := range r.Header {
		if strings.ToLower(name) == "authorization" || strings.ToLower(name) == "x-api-key" {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Make request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward to Claude: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// For streaming responses, we might need to convert back to OpenAI format
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Claude response: %w", err)
	}

	// Convert Claude response back to OpenAI format if needed
	convertedBody := p.convertFromClaudeFormat(responseBody)
	
	_, err = w.Write(convertedBody)
	if err != nil {
		logrus.Errorf("Error writing Claude response: %v", err)
		return err
	}

	return nil
}

func (p *ClaudeProvider) convertToClaudeFormat(requestData map[string]interface{}) []byte {
	claudeRequest := make(map[string]interface{})

	// Set model
	if model, ok := requestData["model"].(string); ok {
		claudeRequest["model"] = model
	} else if p.config.DefaultModel != "" {
		claudeRequest["model"] = p.config.DefaultModel
	} else {
		claudeRequest["model"] = "claude-3-haiku-20240307"
	}

	// Set max_tokens
	if maxTokens, ok := requestData["max_tokens"].(float64); ok {
		claudeRequest["max_tokens"] = int(maxTokens)
	} else {
		claudeRequest["max_tokens"] = 4096
	}

	// Convert messages format
	if messages, ok := requestData["messages"].([]interface{}); ok {
		claudeRequest["messages"] = messages
	}

	// Handle other parameters
	if temp, ok := requestData["temperature"]; ok {
		claudeRequest["temperature"] = temp
	}
	if topP, ok := requestData["top_p"]; ok {
		claudeRequest["top_p"] = topP
	}
	if stream, ok := requestData["stream"]; ok {
		claudeRequest["stream"] = stream
	}

	body, _ := json.Marshal(claudeRequest)
	return body
}

func (p *ClaudeProvider) convertFromClaudeFormat(claudeResponse []byte) []byte {
	// Parse Claude response
	var claudeData map[string]interface{}
	if err := json.Unmarshal(claudeResponse, &claudeData); err != nil {
		// If parsing fails, return as-is
		return claudeResponse
	}

	// Convert to OpenAI format
	openaiResponse := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   claudeData["model"],
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": extractClaudeContent(claudeData),
				},
				"finish_reason": "stop",
			},
		},
	}

	// Add usage information if available
	if usage, ok := claudeData["usage"].(map[string]interface{}); ok {
		openaiResponse["usage"] = usage
	}

	body, _ := json.Marshal(openaiResponse)
	return body
}

func extractClaudeContent(claudeData map[string]interface{}) string {
	if content, ok := claudeData["content"].([]interface{}); ok && len(content) > 0 {
		if item, ok := content[0].(map[string]interface{}); ok {
			if text, ok := item["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

func (p *ClaudeProvider) CalculateCost(inputTokens, outputTokens int) float64 {
	model := p.config.DefaultModel
	if model == "" {
		model = "claude-3-haiku-20240307" // cheapest fallback
	}

	pricing, exists := p.pricing[model]
	if !exists {
		// Use haiku pricing as default
		pricing = p.pricing["claude-3-haiku-20240307"]
	}

	inputCost := float64(inputTokens) * pricing.InputPricePer1K / 1000.0
	outputCost := float64(outputTokens) * pricing.OutputPricePer1K / 1000.0

	return inputCost + outputCost
}

func (p *ClaudeProvider) GetModelPricing() map[string]ModelPricing {
	return p.pricing
}
