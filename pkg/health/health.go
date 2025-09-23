package health

// Most simple health check probe to see whether our server is still alive

import (
    "fmt"
    "net/http"
    "os"
    "strings"

    "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
    "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

func StartHealthServer(port int) chan error {
	errCh := make(chan error)
	go func() {
		sm := http.NewServeMux()
		sm.HandleFunc("/healthz", HealthProbe)
		errCh <- http.ListenAndServe(fmt.Sprintf(":%d", port), sm)
	}()
	return errCh
}

func HealthProbe(w http.ResponseWriter, r *http.Request) {
    log.Tracef("/healthz ping request received, replying with pong")
    // optional fail-open behavior on detected port exhaustion to trigger pod restart
    if shouldFailOnPortExhaustion() && registry.IsPortExhaustionDegraded() {
        w.WriteHeader(http.StatusServiceUnavailable)
        if _, err := w.Write([]byte("PORT-EXHAUSTION")); err != nil {
            log.Errorf("/healthz write failed: %v", err)
        }
        return
    }
    fmt.Fprintf(w, "OK\n")
}

func shouldFailOnPortExhaustion() bool {
    v := strings.ToLower(os.Getenv("HEALTH_FAIL_ON_PORT_EXHAUSTION"))
    if v == "" {
        // Default: enabled (fail liveness on sustained port exhaustion)
        return true
    }
    return v == "1" || v == "true" || v == "yes"
}
