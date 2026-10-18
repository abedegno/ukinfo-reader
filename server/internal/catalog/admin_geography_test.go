package catalog

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopGeographyRequiresCapabilityAndLocalSelection(t *testing.T) {
	admin := &Admin{Token: strings.Repeat("f", 64), Host: "127.0.0.1"}
	for _, c := range []struct {
		path, body string
		authorized bool
		status     int
	}{
		{"/geography-source", "", false, 403},
		{"/import-geography", `{"path":"/tmp/source.zip"}`, false, 403},
		{"/geography-source", "", true, 200},
		{"/import-geography", `{"path":"relative.zip"}`, true, 400},
		{"/import-geography", `{"path":"/tmp/a.zip","sha256":"override"}`, true, 400},
		{"/import-geography", `{"path":"/tmp/a.zip"} {}`, true, 400},
	} {
		method := "POST"
		if c.path == "/geography-source" {
			method = "GET"
		}
		request := httptest.NewRequest(method, "http://127.0.0.1"+c.path, strings.NewReader(c.body))
		request.Header.Set("Content-Type", "application/json")
		if c.authorized {
			request.Header.Set("Authorization", "Bearer "+admin.Token)
		}
		response := httptest.NewRecorder()
		admin.Handler().ServeHTTP(response, request)
		if response.Code != c.status {
			t.Fatalf("%s %s: %d %s", c.path, c.body, response.Code, response.Body.String())
		}
	}
	// The wrong ZIP must be rejected before opening a database transaction.
	file := filepath.Join(t.TempDir(), "wrong.zip")
	if err := os.WriteFile(file, []byte("invented, not ONSPD"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "http://127.0.0.1/import-geography", strings.NewReader(`{"path":"`+file+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+admin.Token)
	response := httptest.NewRecorder()
	admin.Handler().ServeHTTP(response, request)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "SHA-256 mismatch") {
		t.Fatal(response.Code, response.Body.String())
	}
	admin.mu.Lock()
	defer admin.mu.Unlock()
	if admin.state.Running || admin.cancel != nil {
		t.Fatal("failed import retained ownership")
	}
}

func TestPostcodeHashHonorsCancellation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(file, []byte("invented"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fileSHA256Context(ctx, file); err != context.Canceled {
		t.Fatal(err)
	}
}
