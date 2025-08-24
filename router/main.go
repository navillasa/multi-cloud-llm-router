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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Config represents the router configuration
type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Clusters []ClusterConfig `yaml:"clusters"`
	Router   RouterConfig    `yaml:"router"`
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
	StickinessWindow      time.Duration `yaml:"stickinessWindow"`
	HealthCheckInterval   time.Duration `yaml:"healthCheckInterval"`
	MaxLatencyMs          int           `yaml:"maxLatencyMs"`
	MaxQueueDepth         int           `yaml:"maxQueueDepth"`
	OverheadFactor        float64       `yaml:"overheadFactor"`
	MetricsUpdateInterval time.Duration `yaml:"metricsUpdateInterval"`
}

// Router holds the main application state
type Router struct {
	config        *Config
	healthChecker *health.Checker
	costEngine    *cost.Engine
	forwarder     *forward.Forwarder
	metrics       *Metrics
}

// Metrics holds Prometheus metrics
type Metrics struct {
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	clusterHealth    *prometheus.GaugeVec
	clusterCost      *prometheus.GaugeVec
	routingDecisions *prometheus.CounterVec
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
			[]string{"cluster", "reason"},
		),
	}

	prometheus.MustRegister(
		m.requestsTotal,
		m.requestDuration,
		m.clusterHealth,
		m.clusterCost,
		m.routingDecisions,
	)

	return m
}

// NewRouter creates a new router instance
func NewRouter(config *Config) *Router {
	metrics := newMetrics()

	healthChecker := health.NewChecker(config.Router.HealthCheckInterval)
	costEngine := cost.NewEngine(config.Router.OverheadFactor)
	forwarder := forward.NewForwarder()

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

	return &Router{
		config:        config,
		healthChecker: healthChecker,
		costEngine:    costEngine,
		forwarder:     forwarder,
		metrics:       metrics,
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

func (r *Router) selectCluster() (string, string, error) {
	healthyMetrics := r.healthChecker.GetHealthyMetrics()
	if len(healthyMetrics) == 0 {
		return "", "", fmt.Errorf("no healthy clusters available")
	}

	// Filter by SLA constraints
	validClusters := make(map[string]health.ClusterMetrics)
	for name, metrics := range healthyMetrics {
		if metrics.LatencyP95 <= float64(r.config.Router.MaxLatencyMs) &&
			metrics.QueueDepth <= r.config.Router.MaxQueueDepth {
			validClusters[name] = metrics
		}
	}

	if len(validClusters) == 0 {
		return "", "", fmt.Errorf("no clusters meet SLA requirements")
	}

	// Calculate costs and select cheapest
	cheapestCluster := ""
	lowestCost := float64(999999)
	var reason string

	for name, metrics := range validClusters {
		cost := r.costEngine.CalculateCostPer1KTokens(name, metrics.TokensPerSecond)
		if cost < lowestCost {
			lowestCost = cost
			cheapestCluster = name
			reason = "lowest_cost"
		}
	}

	if cheapestCluster == "" {
		return "", "", fmt.Errorf("failed to select cluster")
	}

	// Get endpoint from config
	var endpoint string
	for _, cluster := range r.config.Clusters {
		if cluster.Name == cheapestCluster {
			endpoint = cluster.Endpoint
			break
		}
	}

	r.metrics.routingDecisions.WithLabelValues(cheapestCluster, reason).Inc()

	return cheapestCluster, endpoint, nil
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

	// Select target cluster
	clusterName, clusterEndpoint, err := r.selectCluster()
	if err != nil {
		http.Error(w, fmt.Sprintf("No available clusters: %v", err), http.StatusServiceUnavailable)
		r.metrics.requestsTotal.WithLabelValues("none", "503").Inc()
		return
	}

	// Forward request
	err = r.forwarder.Forward(w, req, clusterName, clusterEndpoint+endpoint)

	// Record metrics
	duration := time.Since(start).Seconds()
	r.metrics.requestDuration.WithLabelValues(clusterName).Observe(duration)

	if err != nil {
		logrus.Errorf("Failed to forward request to %s: %v", clusterName, err)
		r.metrics.requestsTotal.WithLabelValues(clusterName, "error").Inc()
	} else {
		r.metrics.requestsTotal.WithLabelValues(clusterName, "success").Inc()
	}
}

func (r *Router) healthHandler(w http.ResponseWriter, req *http.Request) {
	healthyCount := len(r.healthChecker.GetHealthyMetrics())

	status := map[string]interface{}{
		"status":           "healthy",
		"healthy_clusters": healthyCount,
		"total_clusters":   len(r.config.Clusters),
		"timestamp":        time.Now().Format(time.RFC3339),
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
	allMetrics := r.healthChecker.GetAllMetrics()

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
