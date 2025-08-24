package cost

import (
	"math"
	"sync"
	"time"
)

// Engine calculates and tracks cluster costs
type Engine struct {
	mu             sync.RWMutex
	clusters       map[string]*ClusterCost
	overheadFactor float64
}

// ClusterCost holds cost tracking data for a cluster
type ClusterCost struct {
	CostPerHour      float64
	LastTokensPerSec float64
	LastUpdate       time.Time
	HistoricalCosts  []float64
}

// NewEngine creates a new cost calculation engine
func NewEngine(overheadFactor float64) *Engine {
	return &Engine{
		clusters:       make(map[string]*ClusterCost),
		overheadFactor: overheadFactor,
	}
}

// AddCluster adds a new cluster for cost tracking
func (e *Engine) AddCluster(name string, costPerHour float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.clusters[name] = &ClusterCost{
		CostPerHour:     costPerHour,
		HistoricalCosts: make([]float64, 0, 100), // Keep last 100 calculations
	}
}

// CalculateCostPer1KTokens calculates the effective cost per 1K tokens for a cluster
// Formula: $per1K = (node_hourly_cost / (tokens_per_sec * 3600)) * overhead_factor * 1000
func (e *Engine) CalculateCostPer1KTokens(clusterName string, tokensPerSecond float64) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	cluster, exists := e.clusters[clusterName]
	if !exists {
		return math.Inf(1) // Return infinity for unknown clusters
	}
	
	if tokensPerSecond <= 0 {
		return math.Inf(1) // Can't calculate cost with zero throughput
	}
	
	// Calculate cost per 1K tokens
	tokensPerHour := tokensPerSecond * 3600
	costPer1KTokens := (cluster.CostPerHour / tokensPerHour) * e.overheadFactor * 1000
	
	// Update tracking data
	cluster.LastTokensPerSec = tokensPerSecond
	cluster.LastUpdate = time.Now()
	
	// Store historical data (keep last 100 entries)
	cluster.HistoricalCosts = append(cluster.HistoricalCosts, costPer1KTokens)
	if len(cluster.HistoricalCosts) > 100 {
		cluster.HistoricalCosts = cluster.HistoricalCosts[1:]
	}
	
	return costPer1KTokens
}

// GetClusterCost returns the last calculated cost for a cluster
func (e *Engine) GetClusterCost(clusterName string) (float64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	cluster, exists := e.clusters[clusterName]
	if !exists || len(cluster.HistoricalCosts) == 0 {
		return 0, false
	}
	
	return cluster.HistoricalCosts[len(cluster.HistoricalCosts)-1], true
}

// GetAverageCost returns the average cost over the last N calculations
func (e *Engine) GetAverageCost(clusterName string, lastN int) (float64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	cluster, exists := e.clusters[clusterName]
	if !exists || len(cluster.HistoricalCosts) == 0 {
		return 0, false
	}
	
	costs := cluster.HistoricalCosts
	if lastN > len(costs) {
		lastN = len(costs)
	}
	
	sum := 0.0
	start := len(costs) - lastN
	for i := start; i < len(costs); i++ {
		sum += costs[i]
	}
	
	return sum / float64(lastN), true
}

// UpdateClusterCost updates the hourly cost for a cluster
func (e *Engine) UpdateClusterCost(clusterName string, newCostPerHour float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if cluster, exists := e.clusters[clusterName]; exists {
		cluster.CostPerHour = newCostPerHour
	}
}

// GetAllClusterCosts returns current cost information for all clusters
func (e *Engine) GetAllClusterCosts() map[string]ClusterCostInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	result := make(map[string]ClusterCostInfo)
	for name, cluster := range e.clusters {
		info := ClusterCostInfo{
			CostPerHour:      cluster.CostPerHour,
			LastTokensPerSec: cluster.LastTokensPerSec,
			LastUpdate:       cluster.LastUpdate,
		}
		
		if len(cluster.HistoricalCosts) > 0 {
			info.LastCostPer1K = cluster.HistoricalCosts[len(cluster.HistoricalCosts)-1]
		}
		
		if len(cluster.HistoricalCosts) >= 10 {
			info.AvgCostPer1K, _ = e.getAverageCostUnsafe(cluster, 10)
		}
		
		result[name] = info
	}
	
	return result
}

// ClusterCostInfo provides cost information for a cluster
type ClusterCostInfo struct {
	CostPerHour      float64   `json:"cost_per_hour"`
	LastTokensPerSec float64   `json:"last_tokens_per_sec"`
	LastCostPer1K    float64   `json:"last_cost_per_1k"`
	AvgCostPer1K     float64   `json:"avg_cost_per_1k"`
	LastUpdate       time.Time `json:"last_update"`
}

// Helper function for internal use (assumes lock is held)
func (e *Engine) getAverageCostUnsafe(cluster *ClusterCost, lastN int) (float64, bool) {
	costs := cluster.HistoricalCosts
	if lastN > len(costs) {
		lastN = len(costs)
	}
	
	if lastN == 0 {
		return 0, false
	}
	
	sum := 0.0
	start := len(costs) - lastN
	for i := start; i < len(costs); i++ {
		sum += costs[i]
	}
	
	return sum / float64(lastN), true
}
