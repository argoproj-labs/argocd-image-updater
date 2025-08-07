package webhook

import (
	"sync"
	"time"
)

// RateLimiter implements a sliding window rate limiting algorithm
type RateLimiter struct {
	// mutex for concurrent writing to data
	mu sync.Mutex
	// A map of clients and the timestamps of their requests
	// The key will be an IP address of the client different ports
	// count as a different client
	clients map[string][]int64
	// A map of clients and the timestamp when they were last seen
	// Used for clean up.
	lastSeen map[string]time.Time
	// The window of time checked from the time the request was made
	// Example: If request was made at 12:05 and the window is 5m then
	// then requests between 12:00 - 12:05 will count towards total
	window time.Duration
	// How many requests are allowed in a window
	allowed int
	// A channel used to cancel the clean up go routine
	done chan bool
}

func NewRateLimiter(numRequests int, window time.Duration, cleanUpInterval time.Duration) *RateLimiter {
	limiter := RateLimiter{
		clients:  make(map[string][]int64),
		lastSeen: make(map[string]time.Time),
		window:   window,
		allowed:  numRequests,
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
	for i, ts := range rl.clients[clientIP] {
		if ts > windowStart {
			filtered = rl.clients[clientIP][i:]
			break
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
