package registry

import (
    "errors"
    "net"
    "strings"
    "sync"
    "syscall"
    "time"

    "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
)

// port exhaustion detector: tracks recent EADDRNOTAVAIL dial errors in a sliding window
var peState struct {
    mu        sync.Mutex
    events    []time.Time
    window    time.Duration
    threshold int
    inited    bool
}

func initPortExhaustion() {
    if peState.inited {
        return
    }
    peState.window = env.ParseDurationFromEnv("PORT_EXHAUSTION_WINDOW", 60*time.Second, 1*time.Second, 24*time.Hour)
    peState.threshold = env.ParseNumFromEnv("PORT_EXHAUSTION_THRESHOLD", 8, 1, 100000)
    peState.inited = true
}

func recordPortExhaustionEvent() {
    initPortExhaustion()
    now := time.Now()
    peState.mu.Lock()
    defer peState.mu.Unlock()
    peState.events = append(peState.events, now)
    cutoff := now.Add(-peState.window)
    // drop old events
    i := 0
    for ; i < len(peState.events); i++ {
        if peState.events[i].After(cutoff) {
            break
        }
    }
    if i > 0 && i <= len(peState.events) {
        peState.events = append([]time.Time(nil), peState.events[i:]...)
    }
}

// IsPortExhaustionDegraded returns true if the number of recent EADDRNOTAVAIL
// events within the configured window exceeds the threshold
func IsPortExhaustionDegraded() bool {
    initPortExhaustion()
    now := time.Now()
    cutoff := now.Add(-peState.window)
    peState.mu.Lock()
    defer peState.mu.Unlock()
    // prune
    i := 0
    for ; i < len(peState.events); i++ {
        if peState.events[i].After(cutoff) {
            break
        }
    }
    if i > 0 && i <= len(peState.events) {
        peState.events = append([]time.Time(nil), peState.events[i:]...)
    }
    return len(peState.events) >= peState.threshold
}

// MaybeRecordPortExhaustion checks an error for EADDRNOTAVAIL and records it
func MaybeRecordPortExhaustion(err error) {
    if err == nil {
        return
    }
    // unwrap errors to find syscall codes
    unwrapped := err
    for unwrapped != nil {
        var opErr *net.OpError
        if errors.As(unwrapped, &opErr) {
            // on some systems, opErr.Err can be syscall.Errno
            if errno, ok := opErr.Err.(syscall.Errno); ok && errno == syscall.EADDRNOTAVAIL {
                recordPortExhaustionEvent()
                return
            }
        }
        unwrapped = errors.Unwrap(unwrapped)
    }
    // fallback to substring match
    msg := err.Error()
    if strings.Contains(strings.ToLower(msg), "cannot assign requested address") {
        recordPortExhaustionEvent()
    }
}


