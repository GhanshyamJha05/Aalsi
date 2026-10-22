package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/demo"
	"github.com/aalsi-app/aalsi/pkg/extractor"
)

func TestHarness_Execution(t *testing.T) {
	tempDemoDir := t.TempDir()
	now := time.Now()

	if err := demo.GenerateDemoData(tempDemoDir, now); err != nil {
		t.Fatalf("GenerateDemoData failed: %v", err)
	}

	// Create mock Ollama server for Gemma
	mockGemmaResponses := `[
		{
			"kind": "assignment",
			"title": "OS Assignment 1",
			"subject_or_course": "Operating Systems",
			"raw_date_expression": "` + now.AddDate(0, 0, -7).Format("02/01/2006") + `",
			"raw_time_expression": "5 PM",
			"source_message_ids": ["msg-1"],
			"confidence": 0.95
		},
		{
			"kind": "exam",
			"title": "Mathematics quiz",
			"subject_or_course": "Mathematics",
			"raw_date_expression": "Friday",
			"raw_time_expression": "10 AM",
			"source_message_ids": ["msg-2"],
			"confidence": 0.90
		},
		{
			"kind": "fee",
			"title": "Semester Exam Fee",
			"subject_or_course": "Accounts",
			"raw_date_expression": "` + now.AddDate(0, 0, 5).Format("02 Jan") + `",
			"source_message_ids": ["msg-3"],
			"confidence": 0.95
		},
		{
			"kind": "assignment",
			"title": "DBMS Lab assignment 1",
			"subject_or_course": "DBMS",
			"raw_date_expression": "kal",
			"raw_time_expression": "10:00",
			"source_message_ids": ["msg-4"],
			"confidence": 0.90
		},
		{
			"kind": "assignment",
			"title": "OS Assignment 2",
			"subject_or_course": "Operating Systems",
			"raw_date_expression": "` + now.AddDate(0, 0, 6).Format("02 Jan") + `",
			"raw_time_expression": "5 PM",
			"source_message_ids": ["msg-5"],
			"confidence": 0.95
		},
		{
			"kind": "cancellation",
			"title": "CN lab viva",
			"subject_or_course": "Computer Networks",
			"raw_date_expression": "` + now.AddDate(0, 0, 2).Format("2006-01-02") + `",
			"source_message_ids": ["msg-6"],
			"confidence": 0.95
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			w.Write([]byte(`{"models":[{"name":"gemma3:12b"}]}`))
			return
		}
		w.Write([]byte(`{"message":{"role":"assistant","content":` + string(mustJSON(mockGemmaResponses)) + `}}`))
	}))
	defer srv.Close()

	ext := extractor.New(extractor.Config{
		BaseURL: srv.URL,
		Model:   "gemma3:12b",
		Client:  srv.Client(),
	}, "test prompt:\n{{MESSAGES}}")

	harness := NewHarness(tempDemoDir)
	if err := harness.Run(context.Background(), ext); err != nil {
		t.Fatalf("Harness.Run failed: %v", err)
	}

	resultsPath := filepath.Join(tempDemoDir, "RESULTS.md")
	failuresPath := filepath.Join(tempDemoDir, "failures.md")

	if err := harness.WriteReports(resultsPath, failuresPath); err != nil {
		t.Fatalf("WriteReports failed: %v", err)
	}

	data, err := os.ReadFile(resultsPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("RESULTS.md not written or empty")
	}
}

func mustJSON(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}
