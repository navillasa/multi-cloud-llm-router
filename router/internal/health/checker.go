package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ClusterMetrics holds health and performance metrics for a cluster
type ClusterMetrics struct {
	Healthy          bool      `json:"healthy"`
	LastCheck        time.Time `json:"last_check"`
	ResponseTime     float64   `json:"response_time_ms"`
	LatencyP95       float64   `json:"latency_p95_ms"`
	QueueDepth       int       `json:"queue_depth"`
	TokensPerSecond  float64   `json:"tokens_per_second"`
	ErrorCount       int       `json:"error_count"`
	ConsecutiveError int       `json:"consecutive_errors"`
	Endpoint         string    `json:"endpoint"`
}

// Checker monitors cluster health and collects metrics
type Checker struct {
	mu               sync.RWMutex
	clusters         map[string]*ClusterMetrics
	checkInterval    time.Duration
	httpClient       *http.Client
	maxConsecutiveErrors int
}

// NewChecker creates a new health checker
func NewChecker(checkInterval time.Duration) *Checker {
	return &Checker{
		clusters:             make(map[string]*ClusterMetrics),
		checkInterval:        checkInterval,
		maxConsecutiveErrors: 3,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AddCluster adds a cluster to be monitored
func (c *Checker) AddCluster(name, endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.clusters[name] = &ClusterMetrics{
		Healthy:   false,
		Endpoint:  endpoint,
		LastCheck: time.Now(),
	}
}

// Start begins the health checking loop
func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()
	
	// Initial check
	c.checkAllClusters()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAllClusters()
		}
	}
}

// GetHealthyMetrics returns metrics for only healthy clusters
func (c *Checker) GetHealthyMetrics() map[string]ClusterMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	healthy := make(map[string]ClusterMetrics)
	for name, metrics := range c.clusters {
		if metrics.Healthy {
			healthy[name] = *metrics
		}
	}
	
	return healthy
}

// GetAllMetrics returns metrics for all clusters
func (c *Checker) GetAllMetrics() map[string]ClusterMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	all := make(map[string]ClusterMetrics)
	for name, metrics := range c.clusters {
		all[name] = *metrics
	}
	
	return all
}

// GetClusterMetrics returns metrics for a specific cluster
func (c *Checker) GetClusterMetrics(name string) (ClusterMetrics, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	metrics, exists := c.clusters[name]
	if !exists {
		return ClusterMetrics{}, false
	}
	
	return *metrics, true
}

func (c *Checker) checkAllClusters() {
	c.mu.RLock()
	clusterNames := make([]string, 0, len(c.clusters))
	for name := range c.clusters {
		clusterNames = append(clusterNames, name)
	}
	c.mu.RUnlock()
	
	// Check clusters concurrently
	var wg sync.WaitGroup
	for _, name := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkCluster(clusterName)
		}(name)
	}
	wg.Wait()
}

func (c *Checker) checkCluster(name string) {
	c.mu.RLock()
	cluster, exists := c.clusters[name]
	if !exists {
		c.mu.RUnlock()
		return
	}
	endpoint := cluster.Endpoint
	c.mu.RUnlock()
	
	start := time.Now()
	healthy, queueDepth, tokensPerSec, latencyP95 := c.performHealthCheck(endpoint)
	responseTime := float64(time.Since(start).Nanoseconds()) / 1e6 // Convert to milliseconds
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	cluster = c.clusters[name] // Re-get after acquiring write lock
	cluster.LastCheck = time.Now()
	cluster.ResponseTime = responseTime
	cluster.LatencyP95 = latencyP95
	cluster.QueueDepth = queueDepth
	cluster.TokensPerSecond = tokensPerSec
	
	if healthy {
		cluster.Healthy = true
		cluster.ConsecutiveError = 0
		logrus.Debugf("Cluster %s is healthy (response: %.2fms, tps: %.2f, queue: %d)", 
			name, responseTime, tokensPerSec, queueDepth)
	} else {
		cluster.ErrorCount++
		cluster.ConsecutiveError++
		
		if cluster.ConsecutiveError >= c.maxConsecutiveErrors {
			cluster.Healthy = false
			logrus.Warnf("Cluster %s marked unhealthy after %d consecutive errors", 
				name, cluster.ConsecutiveError)
		}
	}
}

func (c *Checker) performHealthCheck(endpoint string) (healthy bool, queueDepth int, tokensPerSec, latencyP95 float64) {
	// Check basic health endpoint
	healthURL := endpoint + "/health"
	resp, err := c.httpClient.Get(healthURL)
	if err != nil {
		logrus.Debugf("Health check failed for %s: %v", endpoint, err)
		return false, 0, 0, 0
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		logrus.Debugf("Health check returned status %d for %s", resp.StatusCode, endpoint)
		return false, 0, 0, 0
	}
	
	// Try to get metrics if available
	queueDepth, tokensPerSec, latencyP95 = c.getMetrics(endpoint)
	
	return true, queueDepth, tokensPerSec, latencyP95
}

func (c *Checker) getMetrics(endpoint string) (queueDepth int, tokensPerSec, latencyP95 float64) {
	// Default values
	queueDepth = 0
	tokensPerSec = 10.0 // Conservative default
	latencyP95 = 1000.0 // Default 1 second
	
	// Try to get actual metrics from the endpoint
	metricsURL := endpoint + "/metrics"
	resp, err := c.httpClient.Get(metricsURL)
	if err != nil {
		return // Use defaults
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return // Use defaults
	}
	
	// Try to parse metrics (this would be prometheus format typically)
	// For now, we'll try a simple JSON endpoint if available
	statsURL := endpoint + "/stats"
	statsResp, err := c.httpClient.Get(statsURL)
	if err != nil {
		return // Use defaults
	}
	defer statsResp.Body.Close()
	
	if statsResp.StatusCode == http.StatusOK {
		var stats struct {
			QueueDepth      int     `json:"queue_depth"`
			TokensPerSecond float64 `json:"tokens_per_second"`
			LatencyP95      float64 `json:"latency_p95_ms"`
		}
		
		if err := json.NewDecoder(statsResp.Body).Decode(&stats); err == nil {
			if stats.QueueDepth >= 0 {
				queueDepth = stats.QueueDepth
			}
			if stats.TokensPerSecond > 0 {
				tokensPerSec = stats.TokensPerSecond
			}
			if stats.LatencyP95 > 0 {
				latencyP95 = stats.LatencyP95
			}
		}
	}
	
	return queueDepth, tokensPerSec, latencyP95
}

// MarkUnhealthy manually marks a cluster as unhealthy
func (c *Checker) MarkUnhealthy(name string, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if cluster, exists := c.clusters[name]; exists {
		cluster.Healthy = false
		cluster.ErrorCount++
		cluster.ConsecutiveError++
		logrus.Warnf("Cluster %s manually marked unhealthy: %s", name, reason)
	}
}

// ForceHealthy manually marks a cluster as healthy (for testing/admin override)
func (c *Checker) ForceHealthy(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if cluster, exists := c.clusters[name]; exists {
		cluster.Healthy = true
		cluster.ConsecutiveError = 0
		logrus.Infof("Cluster %s manually marked healthy", name)
	}
}
