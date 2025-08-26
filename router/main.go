package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/navillasa/multi-cloud-llm-router/router/internal/cost"
	"github.com/navillasa/multi-cloud-llm-router/router/internal/forward"
	"github.com/navillasa/multi-cloud-llm-router/router/internal/health"
	"github.com/navillasa/multi-cloud-llm-router/router/internal/providers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Config represents the router configuration
type Config struct {
	Server            ServerConfig                   `yaml:"server"`
	Clusters          []ClusterConfig                `yaml:"clusters"`
	ExternalProviders []providers.ProviderConfig     `yaml:"externalProviders"`
	Router            RouterConfig                   `yaml:"router"`
	Demo              DemoConfig                     `yaml:"demo"`
}

// DemoConfig holds demo-specific configuration
type DemoConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Password       string        `yaml:"password"`
	SessionTimeout time.Duration `yaml:"sessionTimeout"`
	RateLimitPerIP int           `yaml:"rateLimitPerIP"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"readTimeout"`
	WriteTimeout time.Duration `yaml:"writeTimeout"`
	IdleTimeout  time.Duration `yaml:"idleTimeout"`
}

type ClusterConfig struct {
	Name         string  `yaml:"name"`
	Endpoint     string  `yaml:"endpoint"`
	Region       string  `yaml:"region"`
	Provider     string  `yaml:"provider"`
	CostPerHour  float64 `yaml:"costPerHour"`
	AuthType     string  `yaml:"authType"` // "hmac" or "mtls"
	SharedSecret string  `yaml:"sharedSecret,omitempty"`
	CertFile     string  `yaml:"certFile,omitempty"`
	KeyFile      string  `yaml:"keyFile,omitempty"`
}

type RouterConfig struct {
	StickinessWindow         time.Duration `yaml:"stickinessWindow"`
	HealthCheckInterval      time.Duration `yaml:"healthCheckInterval"`
	MaxLatencyMs             int           `yaml:"maxLatencyMs"`
	MaxQueueDepth            int           `yaml:"maxQueueDepth"`
	OverheadFactor           float64       `yaml:"overheadFactor"`
	MetricsUpdateInterval    time.Duration `yaml:"metricsUpdateInterval"`
	RoutingStrategy          string        `yaml:"routingStrategy"`
	EnableExternalFallback   bool          `yaml:"enableExternalFallback"`
	ClusterCostThreshold     float64       `yaml:"clusterCostThreshold"`
	EnableSmartMocking       bool          `yaml:"enableSmartMocking"`
	MonthlyAPIBudget         float64       `yaml:"monthlyAPIBudget"`
	MockClusterLatency       int           `yaml:"mockClusterLatency"`
	MockClusterCost          float64       `yaml:"mockClusterCost"`
}

// Router holds the main application state
type Router struct {
	config          *Config
	healthChecker   *health.Checker
	costEngine      *cost.Engine
	forwarder       *forward.Forwarder
	providerManager *providers.ProviderManager
	metrics         *Metrics
}

// Metrics holds Prometheus metrics
type Metrics struct {
	requestsTotal       *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	clusterHealth       *prometheus.GaugeVec
	clusterCost         *prometheus.GaugeVec
	providerHealth      *prometheus.GaugeVec
	providerCost        *prometheus.GaugeVec
	routingDecisions    *prometheus.CounterVec
	externalAPIRequests *prometheus.CounterVec
	tokenUsage          *prometheus.CounterVec
}

func newMetrics() *Metrics {
	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_router_requests_total",
				Help: "Total number of requests processed",
			},
			[]string{"cluster", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "llm_router_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"cluster"},
		),
		clusterHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "llm_router_cluster_health",
				Help: "Cluster health status (1=healthy, 0=unhealthy)",
			},
			[]string{"cluster", "provider", "region"},
		),
		clusterCost: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "llm_router_cluster_cost_per_1k_tokens",
				Help: "Estimated cost per 1K tokens for each cluster",
			},
			[]string{"cluster", "provider", "region"},
		),
		routingDecisions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_router_routing_decisions_total",
				Help: "Total routing decisions made",
			},
			[]string{"target", "type", "reason"},
		),
		providerHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "llm_router_provider_health",
				Help: "External provider health status (1=healthy, 0=unhealthy)",
			},
			[]string{"provider", "type"},
		),
		providerCost: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "llm_router_provider_cost_per_1k_tokens",
				Help: "Estimated cost per 1K tokens for external providers",
			},
			[]string{"provider", "model"},
		),
		externalAPIRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_router_external_requests_total",
				Help: "Total requests sent to external providers",
			},
			[]string{"provider", "model", "status"},
		),
		tokenUsage: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_router_tokens_total",
				Help: "Total tokens processed",
			},
			[]string{"provider", "type"}, // type: input, output
		),
	}

	prometheus.MustRegister(
		m.requestsTotal,
		m.requestDuration,
		m.clusterHealth,
		m.clusterCost,
		m.providerHealth,
		m.providerCost,
		m.routingDecisions,
		m.externalAPIRequests,
		m.tokenUsage,
	)

	return m
}

// NewRouter creates a new router instance
func NewRouter(config *Config) *Router {
	metrics := newMetrics()

	healthChecker := health.NewChecker(config.Router.HealthCheckInterval)
	costEngine := cost.NewEngine(config.Router.OverheadFactor)
	forwarder := forward.NewForwarder()
	providerManager := providers.NewProviderManager()

	// Register clusters
	for _, cluster := range config.Clusters {
		healthChecker.AddCluster(cluster.Name, cluster.Endpoint)
		costEngine.AddCluster(cluster.Name, cluster.CostPerHour)

		// Configure authentication
		switch cluster.AuthType {
		case "hmac":
			forwarder.SetHMACAuth(cluster.Name, cluster.SharedSecret)
		case "mtls":
			if cluster.CertFile != "" && cluster.KeyFile != "" {
				forwarder.SetMTLSAuth(cluster.Name, cluster.CertFile, cluster.KeyFile)
			}
		}
	}

	// Register external providers
	for _, providerConfig := range config.ExternalProviders {
		if !providerConfig.Enabled {
			continue
		}

		// Expand environment variables in API key
		apiKey := os.ExpandEnv(providerConfig.APIKey)
		providerConfig.APIKey = apiKey

		var provider providers.Provider
		switch providerConfig.Type {
		case "openai":
			provider = providers.NewOpenAIProvider(providerConfig)
		case "claude":
			provider = providers.NewClaudeProvider(providerConfig)
		case "gemini":
			provider = providers.NewGeminiProvider(providerConfig)
		default:
			logrus.Warnf("Unknown provider type: %s", providerConfig.Type)
			continue
		}

		providerManager.RegisterProvider(provider)
		logrus.Infof("Registered external provider: %s (%s)", providerConfig.Name, providerConfig.Type)
	}

	return &Router{
		config:          config,
		healthChecker:   healthChecker,
		costEngine:      costEngine,
		forwarder:       forwarder,
		providerManager: providerManager,
		metrics:         metrics,
	}
}

// Start starts the router server
func (r *Router) Start(ctx context.Context) error {
	// Start background services
	go r.healthChecker.Start(ctx)
	go r.updateMetrics(ctx)

	// Setup HTTP server
	router := mux.NewRouter()

	// Health endpoint
	router.HandleFunc("/health", r.healthHandler).Methods("GET")

	// Metrics endpoint
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// Demo authentication endpoint
	if r.config.Demo.Enabled {
		router.HandleFunc("/api/auth", r.authHandler).Methods("POST")
	}

	// LLM API endpoints
	api := router.PathPrefix("/v1").Subrouter()
	api.HandleFunc("/chat/completions", r.chatCompletionsHandler).Methods("POST")
	api.HandleFunc("/completions", r.completionsHandler).Methods("POST")
	api.HandleFunc("/embeddings", r.embeddingsHandler).Methods("POST")

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", r.config.Server.Port),
		Handler:      router,
		ReadTimeout:  r.config.Server.ReadTimeout,
		WriteTimeout: r.config.Server.WriteTimeout,
		IdleTimeout:  r.config.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logrus.Infof("Starting router on port %d", r.config.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

// RouteTarget represents a routing target (cluster or external provider)
type RouteTarget struct {
	Name         string
	Type         string  // "cluster" or "provider"
	Endpoint     string
	Cost         float64
	IsHealthy    bool
	LatencyP95   float64
	QueueDepth   int
	Provider     providers.Provider // only for external providers
}

func (r *Router) selectTarget(ctx context.Context) (*RouteTarget, error) {
	targets := r.getAllTargets(ctx)
	
	if len(targets) == 0 {
		return nil, fmt.Errorf("no healthy targets available")
	}

	// Apply routing strategy
	switch r.config.Router.RoutingStrategy {
	case "cost":
		return r.selectByCost(targets), nil
	case "latency":
		return r.selectByLatency(targets), nil
	case "external_first":
		return r.selectExternalFirst(targets), nil
	case "cluster_first":
		return r.selectClusterFirst(targets), nil
	case "hybrid":
		fallthrough
	default:
		return r.selectHybrid(targets), nil
	}
}

func (r *Router) getAllTargets(ctx context.Context) []*RouteTarget {
	var targets []*RouteTarget

	// Add healthy clusters
	healthyMetrics := r.healthChecker.GetHealthyMetrics()
	for name, metrics := range healthyMetrics {
		if metrics.LatencyP95 <= float64(r.config.Router.MaxLatencyMs) &&
			metrics.QueueDepth <= r.config.Router.MaxQueueDepth {
			
			cost := r.costEngine.CalculateCostPer1KTokens(name, metrics.TokensPerSecond)
			endpoint := ""
			for _, cluster := range r.config.Clusters {
				if cluster.Name == name {
					endpoint = cluster.Endpoint
					break
				}
			}

			targets = append(targets, &RouteTarget{
				Name:       name,
				Type:       "cluster",
				Endpoint:   endpoint,
				Cost:       cost,
				IsHealthy:  true,
				LatencyP95: metrics.LatencyP95,
				QueueDepth: metrics.QueueDepth,
			})
		}
	}

	// Add healthy external providers
	for _, provider := range r.providerManager.GetAllProviders() {
		if err := provider.Health(ctx); err == nil {
			// Use estimated cost based on default model
			pricing := provider.GetModelPricing()
			cost := float64(999999) // fallback high cost
			
			// Get cost from default model or cheapest model
			for _, modelPricing := range pricing {
				avgCost := (modelPricing.InputPricePer1K + modelPricing.OutputPricePer1K) / 2
				if avgCost < cost {
					cost = avgCost
				}
			}

			targets = append(targets, &RouteTarget{
				Name:      provider.Name(),
				Type:      "provider",
				Endpoint:  "", // providers handle their own endpoints
				Cost:      cost,
				IsHealthy: true,
				Provider:  provider,
			})
		}
	}

	return targets
}

func (r *Router) selectByCost(targets []*RouteTarget) *RouteTarget {
	if len(targets) == 0 {
		return nil
	}

	cheapest := targets[0]
	for _, target := range targets[1:] {
		if target.Cost < cheapest.Cost {
			cheapest = target
		}
	}

	r.metrics.routingDecisions.WithLabelValues(cheapest.Name, cheapest.Type, "lowest_cost").Inc()
	return cheapest
}

func (r *Router) selectByLatency(targets []*RouteTarget) *RouteTarget {
	if len(targets) == 0 {
		return nil
	}

	// Prefer clusters for latency (external providers have network overhead)
	fastest := targets[0]
	for _, target := range targets[1:] {
		if target.Type == "cluster" && target.LatencyP95 < fastest.LatencyP95 {
			fastest = target
		}
	}

	r.metrics.routingDecisions.WithLabelValues(fastest.Name, fastest.Type, "lowest_latency").Inc()
	return fastest
}

func (r *Router) selectExternalFirst(targets []*RouteTarget) *RouteTarget {
	// Prefer external providers
	for _, target := range targets {
		if target.Type == "provider" {
			r.metrics.routingDecisions.WithLabelValues(target.Name, target.Type, "external_first").Inc()
			return target
		}
	}

	// Fall back to clusters
	if len(targets) > 0 {
		target := targets[0]
		r.metrics.routingDecisions.WithLabelValues(target.Name, target.Type, "cluster_fallback").Inc()
		return target
	}

	return nil
}

func (r *Router) selectClusterFirst(targets []*RouteTarget) *RouteTarget {
	// Prefer clusters
	for _, target := range targets {
		if target.Type == "cluster" {
			r.metrics.routingDecisions.WithLabelValues(target.Name, target.Type, "cluster_first").Inc()
			return target
		}
	}

	// Fall back to external providers
	if len(targets) > 0 {
		target := targets[0]
		r.metrics.routingDecisions.WithLabelValues(target.Name, target.Type, "external_fallback").Inc()
		return target
	}

	return nil
}

func (r *Router) selectHybrid(targets []*RouteTarget) *RouteTarget {
	if len(targets) == 0 {
		return nil
	}

	// Find cheapest cluster under threshold
	var cheapestCluster *RouteTarget
	for _, target := range targets {
		if target.Type == "cluster" && target.Cost <= r.config.Router.ClusterCostThreshold {
			if cheapestCluster == nil || target.Cost < cheapestCluster.Cost {
				cheapestCluster = target
			}
		}
	}

	// Use cluster if found and cost-effective
	if cheapestCluster != nil {
		r.metrics.routingDecisions.WithLabelValues(cheapestCluster.Name, cheapestCluster.Type, "hybrid_cluster").Inc()
		return cheapestCluster
	}

	// Otherwise use cheapest overall target
	cheapest := targets[0]
	for _, target := range targets[1:] {
		if target.Cost < cheapest.Cost {
			cheapest = target
		}
	}

	r.metrics.routingDecisions.WithLabelValues(cheapest.Name, cheapest.Type, "hybrid_cheapest").Inc()
	return cheapest
}

func (r *Router) chatCompletionsHandler(w http.ResponseWriter, req *http.Request) {
	r.handleLLMRequest(w, req, "/v1/chat/completions")
}

func (r *Router) completionsHandler(w http.ResponseWriter, req *http.Request) {
	r.handleLLMRequest(w, req, "/v1/completions")
}

func (r *Router) embeddingsHandler(w http.ResponseWriter, req *http.Request) {
	r.handleLLMRequest(w, req, "/v1/embeddings")
}

func (r *Router) handleLLMRequest(w http.ResponseWriter, req *http.Request, endpoint string) {
	start := time.Now()
	ctx := req.Context()

	// Select target (cluster or external provider)
	target, err := r.selectTarget(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("No available targets: %v", err), http.StatusServiceUnavailable)
		r.metrics.requestsTotal.WithLabelValues("none", "503").Inc()
		return
	}

	// Forward request based on target type
	if target.Type == "cluster" {
		// Forward to cluster
		err = r.forwarder.Forward(w, req, target.Name, target.Endpoint+endpoint)
	} else if target.Type == "provider" {
		// Forward to external provider
		err = target.Provider.Forward(ctx, w, req, endpoint)
		
		// Record external API request
		status := "success"
		if err != nil {
			status = "error"
		}
		r.metrics.externalAPIRequests.WithLabelValues(target.Name, "unknown", status).Inc()
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	r.metrics.requestDuration.WithLabelValues(target.Name).Observe(duration)

	if err != nil {
		logrus.Errorf("Failed to forward request to %s (%s): %v", target.Name, target.Type, err)
		r.metrics.requestsTotal.WithLabelValues(target.Name, "error").Inc()
	} else {
		r.metrics.requestsTotal.WithLabelValues(target.Name, "success").Inc()
	}
}

func (r *Router) authHandler(w http.ResponseWriter, req *http.Request) {
	if !r.config.Demo.Enabled {
		http.Error(w, "Demo mode not enabled", http.StatusNotFound)
		return
	}

	var authReq struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(req.Body).Decode(&authReq); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if authReq.Password == r.config.Demo.Password {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"token":   "demo-session", // In production, use proper JWT
		})
	} else {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
	}
}

func (r *Router) healthHandler(w http.ResponseWriter, req *http.Request) {
	healthyCount := len(r.healthChecker.GetHealthyMetrics())
	
	// Count healthy external providers
	ctx := req.Context()
	healthyProviders := 0
	for _, provider := range r.providerManager.GetAllProviders() {
		if err := provider.Health(ctx); err == nil {
			healthyProviders++
		}
	}

	status := map[string]interface{}{
		"status":            "healthy",
		"healthy_clusters":  healthyCount,
		"total_clusters":    len(r.config.Clusters),
		"healthy_providers": healthyProviders,
		"total_providers":   len(r.config.ExternalProviders),
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (r *Router) updateMetrics(ctx context.Context) {
	ticker := time.NewTicker(r.config.Router.MetricsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshMetrics()
		}
	}
}

func (r *Router) refreshMetrics() {
	ctx := context.Background()
	allMetrics := r.healthChecker.GetAllMetrics()

	// Update cluster metrics
	for _, cluster := range r.config.Clusters {
		metrics, exists := allMetrics[cluster.Name]

		// Update health metric
		if exists && metrics.Healthy {
			r.metrics.clusterHealth.WithLabelValues(cluster.Name, cluster.Provider, cluster.Region).Set(1)
		} else {
			r.metrics.clusterHealth.WithLabelValues(cluster.Name, cluster.Provider, cluster.Region).Set(0)
		}

		// Update cost metric
		if exists && metrics.TokensPerSecond > 0 {
			cost := r.costEngine.CalculateCostPer1KTokens(cluster.Name, metrics.TokensPerSecond)
			r.metrics.clusterCost.WithLabelValues(cluster.Name, cluster.Provider, cluster.Region).Set(cost)
		}
	}

	// Update external provider metrics
	for _, provider := range r.providerManager.GetAllProviders() {
		// Update health metric
		if err := provider.Health(ctx); err == nil {
			r.metrics.providerHealth.WithLabelValues(provider.Name(), "external").Set(1)
		} else {
			r.metrics.providerHealth.WithLabelValues(provider.Name(), "external").Set(0)
		}

		// Update cost metrics for each model
		pricing := provider.GetModelPricing()
		for model, modelPricing := range pricing {
			avgCost := (modelPricing.InputPricePer1K + modelPricing.OutputPricePer1K) / 2
			r.metrics.providerCost.WithLabelValues(provider.Name(), model).Set(avgCost)
		}
	}
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = 30 * time.Second
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = 30 * time.Second
	}
	if config.Server.IdleTimeout == 0 {
		config.Server.IdleTimeout = 60 * time.Second
	}
	if config.Router.StickinessWindow == 0 {
		config.Router.StickinessWindow = 60 * time.Second
	}
	if config.Router.HealthCheckInterval == 0 {
		config.Router.HealthCheckInterval = 30 * time.Second
	}
	if config.Router.MaxLatencyMs == 0 {
		config.Router.MaxLatencyMs = 5000
	}
	if config.Router.MaxQueueDepth == 0 {
		config.Router.MaxQueueDepth = 10
	}
	if config.Router.OverheadFactor == 0 {
		config.Router.OverheadFactor = 1.1
	}
	if config.Router.MetricsUpdateInterval == 0 {
		config.Router.MetricsUpdateInterval = 30 * time.Second
	}
	if config.Router.RoutingStrategy == "" {
		config.Router.RoutingStrategy = "hybrid"
	}
	if config.Router.ClusterCostThreshold == 0 {
		config.Router.ClusterCostThreshold = 0.01
	}

	return &config, nil
}

func main() {
	var configFile = flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Setup logging
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create router
	router := NewRouter(config)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logrus.Info("Received shutdown signal")
		cancel()
	}()

	// Start router
	if err := router.Start(ctx); err != nil {
		log.Fatalf("Router failed: %v", err)
	}

	logrus.Info("Router shutdown complete")
}
