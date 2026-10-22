package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// Config holds Ollama connection and model settings
type Config struct {
	BaseURL     string // e.g. "http://127.0.0.1:11434"
	Model       string // "gemma3:12b" default, "gemma3:4b" option
	Temperature float64
	Seed        int
	NumCtx      int
	Client      *http.Client
}

// DefaultConfig returns the recommended production configuration
func DefaultConfig() Config {
	return Config{
		BaseURL:     "http://127.0.0.1:11434",
		Model:       "gemma3:12b",
		Temperature: 0.0,
		Seed:        42,
		NumCtx:      8192,
		Client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// Extractor handles communication with local Gemma via Ollama
type Extractor struct {
	cfg        Config
	promptTmpl string
}

// New creates an Extractor instance
func New(cfg Config, promptTmpl string) *Extractor {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "gemma3:12b"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 120 * time.Second}
	}
	if cfg.NumCtx == 0 {
		cfg.NumCtx = 8192
	}
	return &Extractor{
		cfg:        cfg,
		promptTmpl: promptTmpl,
	}
}

// GetInstalledModels queries Ollama /api/tags and returns installed model names
func (e *Extractor) GetInstalledModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/tags", e.cfg.BaseURL), nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama is not running at %s.\nFix: Start Ollama by running 'ollama serve' in your terminal", e.cfg.BaseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned HTTP %d at %s", resp.StatusCode, e.cfg.BaseURL)
	}

	var tagResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagResp); err != nil {
		return nil, fmt.Errorf("reading Ollama model list: %w", err)
	}

	var names []string
	for _, m := range tagResp.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// CheckModelAvailable checks if target model is present
func (e *Extractor) CheckModelAvailable(ctx context.Context, modelName string) (bool, error) {
	models, err := e.GetInstalledModels(ctx)
	if err != nil {
		return false, err
	}
	for _, m := range models {
		if strings.HasPrefix(m, modelName) {
			return true, nil
		}
	}
	return false, nil
}

// CheckOllamaHealth tests reachability and checks if the configured model is installed
func (e *Extractor) CheckOllamaHealth(ctx context.Context) error {
	models, err := e.GetInstalledModels(ctx)
	if err != nil {
		return err
	}

	hasModel := false
	for _, m := range models {
		if strings.HasPrefix(m, e.cfg.Model) {
			hasModel = true
			break
		}
	}

	if !hasModel {
		return fmt.Errorf("Model '%s' is not installed in Ollama.\nFix: Run 'ollama pull %s' to download it", e.cfg.Model, e.cfg.Model)
	}

	return nil
}

// OllamaChatRequest schema for Ollama /api/chat
type ollamaChatRequest struct {
	Model    string               `json:"model"`
	Messages []ollamaMessage      `json:"messages"`
	Stream   bool                 `json:"stream"`
	Format   interface{}          `json:"format"`
	Options  map[string]interface{} `json:"options"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// JSON schema defining the exact output structure for Gemma
var eventSchema = map[string]interface{}{
	"type": "array",
	"items": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"kind": map[string]interface{}{
				"type": "string",
				"enum": []string{"assignment", "exam", "fee", "event", "notice", "cancellation", "postponement", "other"},
			},
			"title": map[string]interface{}{
				"type": "string",
			},
			"subject_or_course": map[string]interface{}{
				"type": []string{"string", "null"},
			},
			"raw_date_expression": map[string]interface{}{
				"type": "string",
			},
			"raw_time_expression": map[string]interface{}{
				"type": []string{"string", "null"},
			},
			"action_needed": map[string]interface{}{
				"type": []string{"string", "null"},
			},
			"refers_to_earlier": map[string]interface{}{
				"type": []string{"string", "null"},
			},
			"source_message_ids": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"confidence": map[string]interface{}{
				"type": "number",
			},
		},
		"required": []string{"kind", "title", "raw_date_expression", "source_message_ids", "confidence"},
	},
}

// ExtractChunk calls Ollama /api/chat with a chunk of candidate messages
func (e *Extractor) ExtractChunk(ctx context.Context, messages []model.Message) ([]model.RawExtractedEvent, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Format messages into readable lines for Gemma
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s] (ID: %s) %s: %s\n",
			m.Timestamp.Format("2006-01-02 15:04"),
			m.ID,
			m.Sender,
			m.Text,
		))
	}

	userTurnContent := strings.Replace(e.promptTmpl, "{{MESSAGES}}", sb.String(), 1)

	reqBody := ollamaChatRequest{
		Model: e.cfg.Model,
		Messages: []ollamaMessage{
			{Role: "user", Content: userTurnContent},
		},
		Stream: false,
		Format: eventSchema,
		Options: map[string]interface{}{
			"temperature": e.cfg.Temperature,
			"seed":        e.cfg.Seed,
			"num_ctx":     e.cfg.NumCtx,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/chat", e.cfg.BaseURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama connection failed: %w.\nFix: Ensure Ollama is running ('ollama serve')", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding Ollama response: %w", err)
	}

	content := strings.TrimSpace(chatResp.Message.Content)
	if content == "" || content == "[]" {
		return nil, nil
	}

	var events []model.RawExtractedEvent
	if err := json.Unmarshal([]byte(content), &events); err != nil {
		return nil, fmt.Errorf("unmarshalling event schema from model: %w\nOutput was: %s", err, content)
	}

	for i := range events {
		events[i].ExtractionMode = "gemma"
	}

	return events, nil
}

// ChunkAndExtract slices messages into chunks of approximate token limit with overlap
func (e *Extractor) ChunkAndExtract(ctx context.Context, messages []model.Message, chunkTokens, overlapTokens int) ([]model.RawExtractedEvent, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Slicing strategy: ~3,500 tokens (approx 14,000 characters) per chunk with ~500 token overlap
	charLimit := chunkTokens * 4
	if charLimit <= 0 {
		charLimit = 14000
	}
	charOverlap := overlapTokens * 4
	if charOverlap <= 0 {
		charOverlap = 2000
	}

	var allEvents []model.RawExtractedEvent
	startIdx := 0

	for startIdx < len(messages) {
		endIdx := startIdx
		accumulatedChars := 0

		for endIdx < len(messages) {
			accumulatedChars += len(messages[endIdx].Text) + len(messages[endIdx].Sender) + 40
			endIdx++
			if accumulatedChars >= charLimit {
				break
			}
		}

		chunk := messages[startIdx:endIdx]
		events, err := e.ExtractChunk(ctx, chunk)
		if err != nil {
			return nil, err
		}
		allEvents = append(allEvents, events...)

		if endIdx >= len(messages) {
			break
		}

		// Calculate step forward for next chunk taking overlap into account
		charsBack := 0
		stepIdx := endIdx
		for stepIdx > startIdx && charsBack < charOverlap {
			stepIdx--
			charsBack += len(messages[stepIdx].Text) + 40
		}
		if stepIdx <= startIdx {
			startIdx = endIdx // guarantee forward progress
		} else {
			startIdx = stepIdx
		}
	}

	return allEvents, nil
}
