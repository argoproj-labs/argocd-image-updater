package webhook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewRateLimiter ensures that a RateLimiter is created correctly
func TestNewRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(5, 5*time.Second, 10*time.Second)
	assert.NotNil(t, limiter, "Limiter was not nil")
}

func TestRateLimiterAllow(t *testing.T) {
	tests := []struct {
		name        string
		clientIP    string
		numRequests int
		window      time.Duration
		sendAmount  int
		allowed     bool
	}{
		{
			name:        "One request",
			clientIP:    "127.0.0.1",
			numRequests: 5,
			window:      5 * time.Second,
			sendAmount:  1,
			allowed:     true,
		},
		{
			name:        "Make some amount of requests but not going obver limit",
			clientIP:    "127.0.0.1",
			numRequests: 50,
			window:      5 * time.Second,
			sendAmount:  20,
			allowed:     true,
		},
		{
			name:        "Go over interval",
			clientIP:    "127.0.0.1",
			numRequests: 5,
			window:      10 * time.Second,
			sendAmount:  10,
			allowed:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CleanUpInterval is set to something large because we are not testing that.
			limiter := NewRateLimiter(tt.numRequests, tt.window, 60*time.Second)

			var allow bool
			for i := 0; i < tt.sendAmount; i++ {
				allow = limiter.Allow(tt.clientIP)
			}
			assert.Equal(t, allow, tt.allowed, "Expected allow to be %v but got %v", tt.allowed, allow)
		})
	}
}

func TestRateLimiterAllowWithWait(t *testing.T) {
	tests := []struct {
		name          string
		clientIP      string
		numRequests   int
		window        time.Duration
		sendStart     int
		sendAfter     int
		allowedBefore bool
		allowedAfter  bool
		waitAmount    time.Duration
	}{
		{
			name:          "Send less that limit before and send less then limit after",
			clientIP:      "127.0.0.1",
			numRequests:   3,
			window:        1 * time.Second,
			sendStart:     1,
			sendAfter:     1,
			allowedBefore: true,
			allowedAfter:  true,
			waitAmount:    2 * time.Second,
		},
		{
			name:          "Send more then limit before and send less then limit after",
			clientIP:      "127.0.0.1",
			numRequests:   5,
			window:        1 * time.Second,
			sendStart:     6,
			sendAfter:     3,
			allowedBefore: false,
			allowedAfter:  true,
			waitAmount:    2 * time.Second,
		},
		{
			name:          "Send less then limit before and send more then limit after",
			clientIP:      "127.0.0.1",
			numRequests:   4,
			window:        1 * time.Second,
			sendStart:     1,
			sendAfter:     5,
			allowedBefore: true,
			allowedAfter:  false,
			waitAmount:    2 * time.Second,
		},
		{
			name:          "Send more then limit before and send more then limit after",
			clientIP:      "127.0.0.1",
			numRequests:   1,
			window:        1 * time.Second,
			sendStart:     5,
			sendAfter:     10,
			allowedBefore: false,
			allowedAfter:  false,
			waitAmount:    2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CleanUpInterval is set to something large because we are not testing that
			limiter := NewRateLimiter(tt.numRequests, tt.window, 60*time.Second)

			var allow bool
			for i := 0; i < tt.sendStart; i++ {
				allow = limiter.Allow(tt.clientIP)
			}
			assert.Equal(t, allow, tt.allowedBefore, "Expected allow to be %v before waiting but got %v", tt.allowedBefore, allow)

			time.Sleep(tt.waitAmount)

			for i := 0; i < tt.sendAfter; i++ {
				allow = limiter.Allow(tt.clientIP)
			}
			assert.Equal(t, allow, tt.allowedAfter, "Expected allow to be %v after waiting but got %v", tt.allowedAfter, allow)
		})
	}
}

func TestRateLimiterAllowMultipleClients(t *testing.T) {
	tests := []struct {
		name    string
		clients []struct {
			IP         string
			sendAmount int
			allowed    bool
		}
		numRequests int
		window      time.Duration
	}{
		{
			name: "Multiple clients that are all allowed",
			clients: []struct {
				IP         string
				sendAmount int
				allowed    bool
			}{
				{IP: "192.168.0.1", sendAmount: 1, allowed: true},
				{IP: "192.168.0.2", sendAmount: 2, allowed: true},
				{IP: "192.168.0.3", sendAmount: 3, allowed: true},
			},
			numRequests: 5,
			window:      2 * time.Second,
		},
		{
			name: "Multiple clients some are allowed some aren't",
			clients: []struct {
				IP         string
				sendAmount int
				allowed    bool
			}{
				{IP: "192.168.0.1", sendAmount: 1, allowed: true},
				{IP: "192.168.0.2", sendAmount: 2, allowed: true},
				{IP: "192.168.0.3", sendAmount: 3, allowed: true},
				{IP: "192.168.0.4", sendAmount: 4, allowed: true},
				{IP: "192.168.0.5", sendAmount: 5, allowed: true},
				{IP: "192.168.0.6", sendAmount: 6, allowed: false},
				{IP: "192.168.0.7", sendAmount: 7, allowed: false},
				{IP: "192.168.0.8", sendAmount: 8, allowed: false},
				{IP: "192.168.0.9", sendAmount: 9, allowed: false},
				{IP: "192.168.0.10", sendAmount: 10, allowed: false},
			},
			numRequests: 5,
			window:      2 * time.Second,
		},
		{
			name: "Multiple clients that are all not allowed",
			clients: []struct {
				IP         string
				sendAmount int
				allowed    bool
			}{
				{IP: "192.168.0.1", sendAmount: 10, allowed: false},
				{IP: "192.168.0.2", sendAmount: 6, allowed: false},
				{IP: "192.168.0.3", sendAmount: 8, allowed: false},
			},
			numRequests: 5,
			window:      2 * time.Second,
		},
		{
			name: "Multiple clients with same IP and different port",
			clients: []struct {
				IP         string
				sendAmount int
				allowed    bool
			}{
				{IP: "127.0.0.1:3000", sendAmount: 2, allowed: true},
				{IP: "127.0.0.1:6443", sendAmount: 10, allowed: false},
				{IP: "127.0.0.1:8080", sendAmount: 5, allowed: true},
			},
			numRequests: 5,
			window:      2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CleanUpInterval is set to something large because we are not testing that
			limiter := NewRateLimiter(tt.numRequests, tt.window, 60*time.Second)

			clientsAllowed := make(map[string]bool)
			for _, client := range tt.clients {
				for i := 0; i < client.sendAmount; i++ {
					clientsAllowed[client.IP] = limiter.Allow(client.IP)
				}

				assert.Equal(t, clientsAllowed[client.IP], client.allowed, "Expected client %s allowed to be %v but got %v", client.IP, client.allowed, clientsAllowed[client.IP])
			}
		})
	}
}

func TestRateLimiterCleanUpKeep(t *testing.T) {
	limiter := NewRateLimiter(5, 5*time.Second, 2*time.Second)
	limiter.Allow("127.0.0.1")

	time.Sleep(3 * time.Second)

	_, existsClients := limiter.clients["127.0.0.1"]
	_, existsLastSeen := limiter.lastSeen["127.0.0.1"]
	assert.True(t, existsClients, "Expected the IP to exist in the clients but it did")
	assert.True(t, existsLastSeen, "Expected the IP to exist in the last seen but it did")
}

func TestRateLimiterCleanUpMultipleClientsKeep(t *testing.T) {
	clients := []string{"192.168.0.1", "192.168.0.2", "192.168.0.3", "192.168.0.4", "192.168.0.5"}

	limiter := NewRateLimiter(5, 5*time.Second, 2*time.Second)
	for _, client := range clients {
		limiter.Allow(client)
	}

	time.Sleep(3 * time.Second)

	for _, client := range clients {
		_, existsClients := limiter.clients[client]
		_, existsLastSeen := limiter.lastSeen[client]
		assert.True(t, existsClients, "Expected the IP, %s, to exist in the clients but it did", client)
		assert.True(t, existsLastSeen, "Expected the IP, %s, to exist in the last seen but it did", client)
	}
}

func TestRateLimiterCleanUpRemove(t *testing.T) {
	limiter := NewRateLimiter(5, 1*time.Second, 2*time.Second)
	limiter.Allow("127.0.0.1")

	time.Sleep(3 * time.Second)

	_, existsClients := limiter.clients["127.0.0.1"]
	_, existsLastSeen := limiter.lastSeen["127.0.0.1"]
	assert.False(t, existsClients, "Expected the IP to not exist in the clients but it did")
	assert.False(t, existsLastSeen, "Expected the IP to not exist in the last seen but it did")
}

func TestRateLimiterCleanUpMultipleClientsRemove(t *testing.T) {
	clients := []string{"192.168.0.1", "192.168.0.2", "192.168.0.3", "192.168.0.4", "192.168.0.5"}

	limiter := NewRateLimiter(5, 1*time.Second, 2*time.Second)
	for _, client := range clients {
		limiter.Allow(client)
	}

	time.Sleep(3 * time.Second)

	for _, client := range clients {
		_, existsClients := limiter.clients[client]
		_, existsLastSeen := limiter.lastSeen[client]
		assert.False(t, existsClients, "Expected the IP, %s, to not exist in the clients but it did", client)
		assert.False(t, existsLastSeen, "Expected the IP, %s, to not exist in the last seen but it did", client)
	}
}

func TestRateLimiterStopCleanUp(t *testing.T) {
	limiter := NewRateLimiter(5, 1*time.Second, 2*time.Second)
	limiter.StopCleanUp()

	limiter.Allow("127.0.0.1")

	time.Sleep(3 * time.Second)

	_, existsClients := limiter.clients["127.0.0.1"]
	_, existsLastSeen := limiter.lastSeen["127.0.0.1"]
	assert.True(t, existsClients, "IP should exist in the clients map because clean up has stopped")
	assert.True(t, existsLastSeen, "IP should exist in the lastSeen map because clean up has stopped")
}
