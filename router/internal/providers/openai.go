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

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config     ProviderConfig
	httpClient *http.Client
	pricing    map[string]ModelPricing
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(config ProviderConfig) *OpenAIProvider {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	provider := &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		pricing: map[string]ModelPricing{
			"gpt-4": {
				InputPricePer1K:  0.03,
				OutputPricePer1K: 0.06,
				MaxTokens:        8192,
				ContextWindow:    8192,
			},
			"gpt-4-turbo": {
				InputPricePer1K:  0.01,
				OutputPricePer1K: 0.03,
				MaxTokens:        4096,
				ContextWindow:    128000,
			},
			"gpt-3.5-turbo": {
				InputPricePer1K:  0.0005,
				OutputPricePer1K: 0.0015,
				MaxTokens:        4096,
				ContextWindow:    16385,
			},
			"gpt-3.5-turbo-16k": {
				InputPricePer1K:  0.003,
				OutputPricePer1K: 0.004,
				MaxTokens:        16384,
				ContextWindow:    16385,
			},
		},
	}

	// Override base URL in config
	provider.config.BaseURL = baseURL
	return provider
}

func (p *OpenAIProvider) Name() string {
	return p.config.Name
}

func (p *OpenAIProvider) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/v1/models", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("User-Agent", "multi-cloud-llm-router/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAI health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func (p *OpenAIProvider) Forward(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint string) error {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	// Parse the request to potentially modify model selection
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		logrus.Warnf("Failed to parse request JSON, forwarding as-is: %v", err)
	} else {
		// Ensure model is set to default if not specified
		if _, hasModel := requestData["model"]; !hasModel && p.config.DefaultModel != "" {
			requestData["model"] = p.config.DefaultModel
			if modifiedBody, err := json.Marshal(requestData); err == nil {
				body = modifiedBody
			}
		}
	}

	// Create target URL
	targetURL := p.config.BaseURL + endpoint
	if !strings.HasPrefix(endpoint, "/") {
		targetURL = p.config.BaseURL + "/" + endpoint
	}

	// Create new request
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Copy relevant headers
	for name, values := range r.Header {
		// Skip authorization headers from client
		if strings.ToLower(name) == "authorization" {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Add OpenAI authentication
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("User-Agent", "multi-cloud-llm-router/1.0")
	req.Header.Set("Content-Type", "application/json")

	// Make request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward to OpenAI: %w", err)
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

	// Stream response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		logrus.Errorf("Error streaming OpenAI response: %v", err)
		return err
	}

	return nil
}

func (p *OpenAIProvider) CalculateCost(inputTokens, outputTokens int) float64 {
	model := p.config.DefaultModel
	if model == "" {
		model = "gpt-3.5-turbo" // fallback
	}

	pricing, exists := p.pricing[model]
	if !exists {
		// Use gpt-3.5-turbo pricing as default
		pricing = p.pricing["gpt-3.5-turbo"]
	}

	inputCost := float64(inputTokens) * pricing.InputPricePer1K / 1000.0
	outputCost := float64(outputTokens) * pricing.OutputPricePer1K / 1000.0

	return inputCost + outputCost
}

func (p *OpenAIProvider) GetModelPricing() map[string]ModelPricing {
	return p.pricing
}

// EstimateTokensFromText provides a rough estimation of tokens
// This is a simplified estimation - in production you'd want to use tiktoken
func (p *OpenAIProvider) EstimateTokensFromText(text string) int {
	// Rough estimation: ~1 token per 4 characters for English text
	return len(text) / 4
}
