package argocd

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// trustServerViaCertStore registers the test server's certificate in a
// temporary Argo CD TLS certificate store keyed by the server's hostname, the
// way an operator registers the private CA of a self-hosted SCM instance.
func trustServerViaCertStore(t *testing.T, server *httptest.Server) {
	t.Helper()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NotNil(t, pemData)
	require.NoError(t, os.WriteFile(filepath.Join(dir, u.Hostname()), pemData, 0o600))
	t.Setenv("ARGOCD_TLS_DATA_PATH", dir)
}

func Test_newSCMAPIHTTPClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	repoURL := server.URL + "/owner/repo.git"

	t.Run("certificate not registered", func(t *testing.T) {
		t.Setenv("ARGOCD_TLS_DATA_PATH", t.TempDir())
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 5*time.Second, nil)
		// The request fails during the TLS handshake, so there is no body.
		_, err := client.Get(server.URL)
		require.ErrorContains(t, err, "certificate signed by unknown authority")
	})

	t.Run("certificate registered in the Argo CD cert store", func(t *testing.T) {
		trustServerViaCertStore(t, server)
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 5*time.Second, nil)
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("timeout and redirect policy come from the caller", func(t *testing.T) {
		checkRedirect := func(_ *http.Request, _ []*http.Request) error { return nil }
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 7*time.Second, checkRedirect)
		require.Equal(t, 7*time.Second, client.Timeout)
		require.NotNil(t, client.CheckRedirect)
	})
}
