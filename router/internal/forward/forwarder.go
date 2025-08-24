package forward

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Forwarder handles request forwarding to clusters with authentication
type Forwarder struct {
	mu          sync.RWMutex
	hmacSecrets map[string]string
	tlsConfigs  map[string]*tls.Config
	httpClient  *http.Client
}

// NewForwarder creates a new request forwarder
func NewForwarder() *Forwarder {
	return &Forwarder{
		hmacSecrets: make(map[string]string),
		tlsConfigs:  make(map[string]*tls.Config),
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Long timeout for LLM generation
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true, // Let the client handle compression
			},
		},
	}
}

// SetHMACAuth configures HMAC authentication for a cluster
func (f *Forwarder) SetHMACAuth(clusterName, sharedSecret string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hmacSecrets[clusterName] = sharedSecret
}

// SetMTLSAuth configures mTLS authentication for a cluster
func (f *Forwarder) SetMTLSAuth(clusterName, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("failed to load client certificate: %w", err)
	}
	
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   clusterName, // Use cluster name as server name
	}
	
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tlsConfigs[clusterName] = tlsConfig
	
	return nil
}

// Forward forwards an HTTP request to the specified cluster endpoint
func (f *Forwarder) Forward(w http.ResponseWriter, r *http.Request, clusterName, targetURL string) error {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()
	
	// Create new request
	req, err := http.NewRequest(r.Method, targetURL, io.NopCloser(bytes.NewBuffer(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Copy headers
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	
	// Add authentication
	f.addAuthentication(req, clusterName, body)
	
	// Configure client for this request
	client := f.getClientForCluster(clusterName)
	
	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward request: %w", err)
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
		logrus.Errorf("Error streaming response from %s: %v", clusterName, err)
		return err
	}
	
	return nil
}

func (f *Forwarder) addAuthentication(req *http.Request, clusterName string, body []byte) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	// Check for HMAC authentication
	if secret, exists := f.hmacSecrets[clusterName]; exists {
		f.addHMACAuth(req, secret, body)
	}
	
	// mTLS is handled by the HTTP client configuration
}

func (f *Forwarder) addHMACAuth(req *http.Request, secret string, body []byte) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	
	// Create signature data: timestamp + method + path + body
	signatureData := timestamp + req.Method + req.URL.Path + string(body)
	
	// Calculate HMAC
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureData))
	signature := hex.EncodeToString(h.Sum(nil))
	
	// Add headers
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Auth-Type", "hmac-sha256")
}

func (f *Forwarder) getClientForCluster(clusterName string) *http.Client {
	f.mu.RLock()
	tlsConfig, hasTLS := f.tlsConfigs[clusterName]
	f.mu.RUnlock()
	
	if !hasTLS {
		return f.httpClient
	}
	
	// Create a client with custom TLS config for this cluster
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	
	return &http.Client{
		Timeout:   120 * time.Second,
		Transport: transport,
	}
}

// ValidateHMACSignature validates an incoming HMAC signature (for server-side validation)
func ValidateHMACSignature(req *http.Request, secret string, body []byte) bool {
	timestamp := req.Header.Get("X-Timestamp")
	signature := req.Header.Get("X-Signature")
	authType := req.Header.Get("X-Auth-Type")
	
	if timestamp == "" || signature == "" || authType != "hmac-sha256" {
		return false
	}
	
	// Check timestamp (prevent replay attacks)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	
	now := time.Now().Unix()
	if abs(now-ts) > 300 { // 5 minute window
		return false
	}
	
	// Recreate signature
	signatureData := timestamp + req.Method + req.URL.Path + string(body)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureData))
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
