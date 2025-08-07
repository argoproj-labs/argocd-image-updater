package webhook

import (
	"sync"
	"time"
)

// RateLimiter implements a sliding window rate limiting algorithm
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string][]int64
	lastSeen map[string]time.Time
	window   time.Duration
	allowed  int
	done     chan bool
}

func NewRateLimiter(numRequets int, window time.Duration, cleanUpInterval time.Duration) *RateLimiter {
	limiter := RateLimiter{
		clients:  make(map[string][]int64),
		lastSeen: make(map[string]time.Time),
		window:   window,
		allowed:  numRequets,
		done:     make(chan bool),
	}
	go limiter.StartCleanUp(cleanUpInterval)
	return &limiter
}

// Checks to see if a client has gone over the limit
func (rl *RateLimiter) Allow(clientIP string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, ok := rl.clients[clientIP]; !ok {
		rl.clients[clientIP] = []int64{}
	}

	allow := false
	windowStart := now.Unix() - int64(rl.window.Seconds())
	filtered := []int64{}
	for _, ts := range rl.clients[clientIP] {
		if ts > windowStart {
			filtered = append(filtered, ts)
		}
	}
	rl.clients[clientIP] = filtered

	if len(filtered) < rl.allowed {
		rl.clients[clientIP] = append(filtered, now.Unix())
		rl.lastSeen[clientIP] = now
		allow = true
	}

	return allow
}

// Cleans up the clients map at an interval to prevent stale clients from taking up memory
func (rl *RateLimiter) StartCleanUp(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			rl.CleanUp()
		}
	}
}

// Cleans up any clients that have not made a request in an amount of time over the window
func (rl *RateLimiter) CleanUp() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for k, v := range rl.lastSeen {
		now := time.Now()
		if v.Add(rl.window).Before(now) {
			delete(rl.clients, k)
			delete(rl.lastSeen, k)
		}
	}
}

// Stop the clean up goroutine
func (rl *RateLimiter) StopCleanUp() {
	rl.done <- true
}
