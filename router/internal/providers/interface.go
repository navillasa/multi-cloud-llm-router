package providers

import (
	"context"
	"net/http"
)

// Provider represents an external LLM provider
type Provider interface {
	// Name returns the provider name (e.g., "openai", "claude", "gemini")
	Name() string
	
	// Health checks if the provider is available
	Health(ctx context.Context) error
	
	// Forward forwards a request to the provider and streams the response
	Forward(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint string) error
	
	// CalculateCost estimates the cost for a request ($/1K tokens)
	CalculateCost(inputTokens, outputTokens int) float64
	
	// GetModelPricing returns pricing information for the provider's models
	GetModelPricing() map[string]ModelPricing
}

// ModelPricing represents pricing information for a model
type ModelPricing struct {
	InputPricePer1K  float64 // Price per 1K input tokens
	OutputPricePer1K float64 // Price per 1K output tokens
	MaxTokens        int     // Maximum tokens supported
	ContextWindow    int     // Context window size
}

// ProviderConfig represents configuration for an external provider
type ProviderConfig struct {
	Name         string            `yaml:"name"`
	Type         string            `yaml:"type"` // "openai", "claude", "gemini"
	APIKey       string            `yaml:"apiKey"`
	BaseURL      string            `yaml:"baseURL,omitempty"`
	DefaultModel string            `yaml:"defaultModel"`
	Enabled      bool              `yaml:"enabled"`
	RateLimit    RateLimitConfig   `yaml:"rateLimit"`
	Models       map[string]string `yaml:"models,omitempty"` // endpoint mapping
}

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int     `yaml:"requestsPerMinute"`
	TokensPerMinute   int     `yaml:"tokensPerMinute"`
	BurstMultiplier   float64 `yaml:"burstMultiplier"`
}

// RequestMetadata holds information about a request for cost calculation
type RequestMetadata struct {
	Model        string
	InputTokens  int
	OutputTokens int
	Provider     string
	Duration     float64
}

// ProviderManager manages multiple external providers
type ProviderManager struct {
	providers map[string]Provider
}

// NewProviderManager creates a new provider manager
func NewProviderManager() *ProviderManager {
	return &ProviderManager{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider registers a new provider
func (pm *ProviderManager) RegisterProvider(provider Provider) {
	pm.providers[provider.Name()] = provider
}

// GetProvider returns a provider by name
func (pm *ProviderManager) GetProvider(name string) (Provider, bool) {
	provider, exists := pm.providers[name]
	return provider, exists
}

// GetAllProviders returns all registered providers
func (pm *ProviderManager) GetAllProviders() map[string]Provider {
	return pm.providers
}

// GetHealthyProviders returns only healthy providers
func (pm *ProviderManager) GetHealthyProviders(ctx context.Context) map[string]Provider {
	healthy := make(map[string]Provider)
	for name, provider := range pm.providers {
		if err := provider.Health(ctx); err == nil {
			healthy[name] = provider
		}
	}
	return healthy
}
