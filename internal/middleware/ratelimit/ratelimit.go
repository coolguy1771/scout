package ratelimit

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coolguy1771/scout/internal/middleware/auth"
)

// RateLimiter implements per-tenant rate limiting
type RateLimiter struct {
	// Global rate limit (requests per second)
	globalLimit float64
	// Per-tenant rate limit (requests per second)
	tenantLimit float64
	// Burst size for token bucket
	burst int
	// Token buckets per tenant
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
	// Cleanup interval for old buckets
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	capacity  float64
	rate      float64
}

// NewRateLimiter creates a new rate limiter
// globalLimit: global rate limit in requests per second
// tenantLimit: per-tenant rate limit in requests per second
// burst: burst size (max tokens in bucket)
func NewRateLimiter(globalLimit, tenantLimit float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		globalLimit:     globalLimit,
		tenantLimit:     tenantLimit,
		burst:           burst,
		buckets:         make(map[string]*tokenBucket),
		cleanupInterval: 5 * time.Minute,
		lastCleanup:     time.Now(),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Middleware returns a rate limiting middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get tenant ID from context
		tenantID := auth.GetTenantID(r.Context())
		if tenantID == "" {
			// No tenant ID, use global limit
			tenantID = "global"
		}

		// Check rate limit
		allowed, remaining, resetTime := rl.Allow(tenantID)
		if !allowed {
			w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(rl.tenantLimit, 'f', 0, 64))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
			w.Header().Set("Retry-After", strconv.FormatInt(int64(time.Until(time.Unix(resetTime, 0)).Seconds()), 10))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(rl.tenantLimit, 'f', 0, 64))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(int64(remaining), 10))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))

		next.ServeHTTP(w, r)
	})
}

// Allow checks if a request is allowed for the given tenant
// Returns: allowed, remaining tokens, reset time (unix timestamp)
func (rl *RateLimiter) Allow(tenantID string) (bool, int, int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket := rl.getBucket(tenantID)
	now := time.Now()

	// Refill tokens based on elapsed time
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	tokensToAdd := elapsed * bucket.rate
	bucket.tokens = min(bucket.capacity, bucket.tokens+tokensToAdd)
	bucket.lastUpdate = now

	// Check if we have enough tokens
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		remaining := int(bucket.tokens)
		resetTime := now.Add(time.Duration(float64(time.Second) * (1.0 / bucket.rate))).Unix()
		return true, remaining, resetTime
	}

	// Not enough tokens
	resetTime := now.Add(time.Duration(float64(time.Second) * ((1.0 - bucket.tokens) / bucket.rate))).Unix()
	return false, 0, resetTime
}

// getBucket gets or creates a token bucket for a tenant
func (rl *RateLimiter) getBucket(tenantID string) *tokenBucket {
	bucket, exists := rl.buckets[tenantID]
	if !exists {
		limit := rl.tenantLimit
		if tenantID == "global" {
			limit = rl.globalLimit
		}
		bucket = &tokenBucket{
			tokens:     float64(rl.burst),
			lastUpdate: time.Now(),
			capacity:   float64(rl.burst),
			rate:       limit,
		}
		rl.buckets[tenantID] = bucket
	}
	return bucket
}

// cleanup removes old buckets to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for tenantID, bucket := range rl.buckets {
			// Remove buckets that haven't been used in 1 hour
			if now.Sub(bucket.lastUpdate) > time.Hour {
				delete(rl.buckets, tenantID)
			}
		}
		rl.lastCleanup = now
		rl.mu.Unlock()
	}
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}



