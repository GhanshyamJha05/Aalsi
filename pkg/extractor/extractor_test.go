package extractor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

func TestExtractor_ExtractChunk_MockOllama(t *testing.T) {
	mockResponseJSON := `[
		{
			"kind": "assignment",
			"title": "OS Assignment 2",
			"subject_or_course": "Operating Systems",
			"raw_date_expression": "18th Oct",
			"raw_time_expression": "5 PM",
			"action_needed": "Submit on portal",
			"refers_to_earlier": null,
			"source_message_ids": ["msg-1"],
			"confidence": 0.95
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat" {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Model != "gemma3:12b" {
				http.Error(w, "unexpected model", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": mockResponseJSON,
				},
			})
			return
		}

		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "gemma3:12b"},
					{"name": "gemma3:4b"},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := Config{
		BaseURL: server.URL,
		Model:   "gemma3:12b",
		Client:  server.Client(),
	}

	ext := New(cfg, "User turn:\n{{MESSAGES}}")

	// 1. Health check
	if err := ext.CheckOllamaHealth(context.Background()); err != nil {
		t.Fatalf("CheckOllamaHealth failed: %v", err)
	}

	// 2. ExtractChunk
	msgs := []model.Message{
		{
			ID:        "msg-1",
			Sender:    "Prof Sharma",
			Timestamp: time.Now(),
			Text:      "Operating Systems assignment 2 is due on 18th Oct by 5 PM.",
		},
	}

	events, err := ext.ExtractChunk(context.Background(), msgs)
	if err != nil {
		t.Fatalf("ExtractChunk failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ev := events[0]
	if ev.Kind != model.KindAssignment {
		t.Errorf("expected KindAssignment, got %s", ev.Kind)
	}
	if ev.Title != "OS Assignment 2" {
		t.Errorf("expected 'OS Assignment 2', got %s", ev.Title)
	}
	if ev.RawDateExpression != "18th Oct" {
		t.Errorf("expected '18th Oct', got %s", ev.RawDateExpression)
	}
	if ev.RawTimeExpression == nil || *ev.RawTimeExpression != "5 PM" {
		t.Errorf("expected '5 PM', got %v", ev.RawTimeExpression)
	}
}

func TestExtractor_FriendlyErrors(t *testing.T) {
	// Connect to non-running port
	cfg := Config{
		BaseURL: "http://127.0.0.1:54321",
		Model:   "gemma3:12b",
		Client:  &http.Client{Timeout: 500 * time.Millisecond},
	}
	ext := New(cfg, "test")

	err := ext.CheckOllamaHealth(context.Background())
	if err == nil {
		t.Fatalf("expected health check to fail on unreachable port")
	}

	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("expected friendly fix command 'ollama serve' in error message: %v", err)
	}
}
