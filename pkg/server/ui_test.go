package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIStaticAssetsAndSpinnerDefaults(t *testing.T) {
	// Setup test server with web directory handler
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("../../web"))
	mux.Handle("/", fs)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Fetch index.html
	res, err := http.Get(ts.URL + "/index.html")
	if err != nil {
		t.Fatalf("Failed to fetch index.html: %v", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Failed to read index.html body: %v", err)
	}
	html := string(bodyBytes)

	// Requirement 1: Spinner must be hidden by default
	if !strings.Contains(html, `id="uploadProgress" class="upload-progress hidden"`) {
		t.Errorf("Expected uploadProgress to have class 'hidden' by default in index.html")
	}

	if !strings.Contains(html, `class="md3-spinner"`) {
		t.Errorf("Expected uploadProgress to contain md3-spinner")
	}

	// 2. Fetch style.css
	resCss, err := http.Get(ts.URL + "/style.css")
	if err != nil {
		t.Fatalf("Failed to fetch style.css: %v", err)
	}
	defer resCss.Body.Close()

	cssBytes, err := io.ReadAll(resCss.Body)
	if err != nil {
		t.Fatalf("Failed to read style.css body: %v", err)
	}
	css := string(cssBytes)

	// Verify .hidden display none rule exists
	if !strings.Contains(css, ".hidden") || !strings.Contains(css, "display: none !important;") {
		t.Errorf("Expected style.css to define .hidden { display: none !important; }")
	}

	// Verify Ollama badge online / offline styling exists
	if !strings.Contains(css, ".md3-chip.online") || !strings.Contains(css, ".md3-chip.offline") {
		t.Errorf("Expected style.css to define online and offline chip classes")
	}

	// Requirement 5: Remove "Material You (MD3) Edition" footer text
	if strings.Contains(html, "Material You (MD3) Edition") {
		t.Errorf("Expected 'Material You (MD3) Edition' to be removed from footer")
	}
	if !strings.Contains(html, "Aalsi &bull; Local &amp; Private") {
		t.Errorf("Expected clean footer 'Aalsi &bull; Local &amp; Private'")
	}

	// Requirement 1 & 2: Verify extractedByMode and fallbackChoiceModal exist in DOM
	if !strings.Contains(html, `id="extractedByMode"`) {
		t.Errorf("Expected #extractedByMode in index.html")
	}
	if !strings.Contains(html, `id="fallbackChoiceModal"`) {
		t.Errorf("Expected #fallbackChoiceModal in index.html")
	}
}
