package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/store"
	"github.com/aalsi-app/aalsi/pkg/tracker"
)

func TestServer_RoutesAndI18n(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "server_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	tr := tracker.New(st)
	srv := NewServer(Config{
		Store:     st,
		Tracker:   tr,
		Extractor: nil,
		Location:  time.FixedZone("IST", 5*3600+30*60),
	})

	// 1. Root HTML
	reqRoot := httptest.NewRequest("GET", "/", nil)
	wRoot := httptest.NewRecorder()
	srv.ServeHTTP(wRoot, reqRoot)
	if wRoot.Code != http.StatusOK {
		t.Errorf("expected 200 on /, got %d", wRoot.Code)
	}
	if !strings.Contains(wRoot.Body.String(), "Aalsi") {
		t.Errorf("expected index.html with Aalsi branding")
	}

	// 2. Health Endpoint
	reqHealth := httptest.NewRequest("GET", "/api/health", nil)
	wHealth := httptest.NewRecorder()
	srv.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/health, got %d", wHealth.Code)
	}

	// 3. Digest Endpoint
	reqDigest := httptest.NewRequest("GET", "/api/digest?lang=en", nil)
	wDigest := httptest.NewRecorder()
	srv.ServeHTTP(wDigest, reqDigest)
	if wDigest.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/digest, got %d", wDigest.Code)
	}

	// 4. Export ICS Endpoint
	reqICS := httptest.NewRequest("GET", "/api/export.ics", nil)
	wICS := httptest.NewRecorder()
	srv.ServeHTTP(wICS, reqICS)
	if wICS.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/export.ics, got %d", wICS.Code)
	}
	if !strings.Contains(wICS.Body.String(), "BEGIN:VCALENDAR") {
		t.Errorf("expected valid VCALENDAR content")
	}

	// 5. Purge Endpoint
	reqPurge := httptest.NewRequest("POST", "/api/purge", nil)
	wPurge := httptest.NewRecorder()
	srv.ServeHTTP(wPurge, reqPurge)
	if wPurge.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/purge, got %d", wPurge.Code)
	}
}

func TestDetectLANIPv4(t *testing.T) {
	// Should either detect valid IPv4 or return clean error without panic
	ip, err := DetectLANIPv4()
	if err == nil {
		if !strings.Contains(ip, ".") {
			t.Errorf("expected dotted IPv4 address, got %s", ip)
		}
	}
}
