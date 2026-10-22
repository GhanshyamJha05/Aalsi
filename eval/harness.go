package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/demo"
	"github.com/aalsi-app/aalsi/pkg/extractor"
	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/parser"
	"github.com/aalsi-app/aalsi/pkg/prefilter"
	"github.com/aalsi-app/aalsi/pkg/store"
	"github.com/aalsi-app/aalsi/pkg/tracker"
)

// EvalResult encapsulates quantitative metrics for a model/configuration run
type EvalResult struct {
	ConfigName             string  `json:"config_name"`
	Mode                   string  `json:"mode"` // "rule" or "gemma"
	TotalTrueEvents        int     `json:"total_true_events"`
	TruePositives          int     `json:"true_positives"`
	FalsePositives         int     `json:"false_positives"`
	FalseNegatives         int     `json:"false_negatives"`
	Precision              float64 `json:"precision"`
	Recall                 float64 `json:"recall"`
	F1Score                float64 `json:"f1_score"`
	DateAccuracy           float64 `json:"date_accuracy"`
	StateAccuracy          float64 `json:"state_accuracy"`
	FalsePositiveNoiseRate float64 `json:"false_positive_noise_rate"`
	ItemsSentToReview      int     `json:"items_sent_to_review"`
	SecondsElapsed         float64 `json:"seconds_elapsed"`
}

// FailureRecord documents every missed or mismatched item
type FailureRecord struct {
	ConfigName string `json:"config_name"`
	ItemID     string `json:"item_id"`
	Title      string `json:"title"`
	Expected   string `json:"expected"`
	Got        string `json:"got"`
	Reason     string `json:"reason"`
}

// Harness executes the evaluation across configurations
type Harness struct {
	demoDir  string
	results  []EvalResult
	failures []FailureRecord
}

// NewHarness creates an evaluation harness
func NewHarness(demoDir string) *Harness {
	return &Harness{demoDir: demoDir}
}

// Run executes evaluation across:
// 1. Rule Mode Baseline (No LLM)
// 2. Gemma Model (Refuses if Ollama is unreachable)
func (h *Harness) Run(ctx context.Context, ext *extractor.Extractor) error {
	// 1. Load ground truth
	gtBytes, err := os.ReadFile(filepath.Join(h.demoDir, "ground_truth.json"))
	if err != nil {
		return fmt.Errorf("reading ground truth: %w", err)
	}

	var groundTruth []demo.GroundTruthItem
	if err := json.Unmarshal(gtBytes, &groundTruth); err != nil {
		return fmt.Errorf("decoding ground truth: %w", err)
	}

	// 2. Load demo chat (Android format)
	chatFile := filepath.Join(h.demoDir, "college_chat_android.txt")
	f, err := os.Open(chatFile)
	if err != nil {
		return fmt.Errorf("opening demo chat: %w", err)
	}
	defer f.Close()

	parsed, err := parser.ParseChat(f, parser.ParseOptions{
		GroupID: "eval-group",
	})
	if err != nil {
		return fmt.Errorf("parsing demo chat: %w", err)
	}

	// Run Configuration 1: Rule Mode (No LLM)
	h.runRegexBaseline(ctx, parsed.Messages, groundTruth)

	// Run Configuration 2: Gemma Model
	if ext != nil {
		// Requirement 3: Refuse to report Gemma results if Ollama is unreachable!
		if err := ext.CheckOllamaHealth(ctx); err != nil {
			return fmt.Errorf("eval harness refused Gemma evaluation: Ollama is unreachable or model not found (%w)", err)
		}
		h.runExtractorConfig(ctx, "Gemma 3 12B", ext, parsed.Messages, groundTruth, 3500, 500)
		h.runExtractorConfig(ctx, "Gemma (No Overlap Ablation)", ext, parsed.Messages, groundTruth, 3500, 0)
	}

	return nil
}

func (h *Harness) runRegexBaseline(ctx context.Context, messages []model.Message, groundTruth []demo.GroundTruthItem) {
	start := time.Now()
	tempDB := filepath.Join(os.TempDir(), fmt.Sprintf("eval_regex_%d.db", time.Now().UnixNano()))
	defer os.Remove(tempDB)

	st, err := store.Open(tempDB)
	if err != nil {
		return
	}
	defer st.Close()

	tr := tracker.New(st)
	msgMap := make(map[string]model.Message)
	for _, m := range messages {
		msgMap[m.ID] = m
	}

	// Pure regex date search without LLM
	candidates := prefilter.FilterMessages(messages)
	for _, m := range candidates {
		// Naive regex detector
		rawEvent := model.RawExtractedEvent{
			Kind:              model.KindAssignment,
			Title:             m.Text,
			RawDateExpression: "today",
			SourceMessageIDs:  []string{m.ID},
			Confidence:        0.50,
		}
		tr.ProcessRawEvent(ctx, "eval-group", rawEvent, msgMap)
	}

	items, _ := st.GetItems(ctx, true)
	elapsed := time.Since(start).Seconds()

	res := h.evaluateItems("Rule Mode Baseline (No LLM)", items, groundTruth, elapsed)
	h.results = append(h.results, res)
}

func (h *Harness) runExtractorConfig(ctx context.Context, name string, ext *extractor.Extractor, messages []model.Message, groundTruth []demo.GroundTruthItem, chunkTokens, overlapTokens int) {
	start := time.Now()
	tempDB := filepath.Join(os.TempDir(), fmt.Sprintf("eval_ext_%d.db", time.Now().UnixNano()))
	defer os.Remove(tempDB)

	st, err := store.Open(tempDB)
	if err != nil {
		return
	}
	defer st.Close()

	tr := tracker.New(st)
	msgMap := make(map[string]model.Message)
	for _, m := range messages {
		msgMap[m.ID] = m
	}

	candidates := prefilter.FilterMessages(messages)
	events, err := ext.ChunkAndExtract(ctx, candidates, chunkTokens, overlapTokens)
	if err == nil {
		for _, ev := range events {
			tr.ProcessRawEvent(ctx, "eval-group", ev, msgMap)
		}
	}

	items, _ := st.GetItems(ctx, true)
	elapsed := time.Since(start).Seconds()

	res := h.evaluateItems(name, items, groundTruth, elapsed)
	h.results = append(h.results, res)
}

func (h *Harness) evaluateItems(configName string, items []model.DeadlineItem, groundTruth []demo.GroundTruthItem, elapsedSec float64) EvalResult {
	tp := 0
	fp := 0
	fn := 0
	dateCorrect := 0
	stateCorrect := 0
	reviewCount := 0

	matchedItems := make(map[string]bool)

	for _, gt := range groundTruth {
		var bestMatch *model.DeadlineItem
		for i := range items {
			it := &items[i]
			if strings.Contains(strings.ToLower(it.Title), strings.ToLower(gt.Subject)) ||
				strings.Contains(strings.ToLower(it.SubjectOrCourse), strings.ToLower(gt.Subject)) ||
				strings.Contains(strings.ToLower(it.Title), strings.ToLower(gt.Title)) {
				bestMatch = it
				matchedItems[it.ID] = true
				break
			}
		}

		if bestMatch != nil {
			tp++
			// Date accuracy check
			if bestMatch.DueDate.Format("2006-01-02") == gt.ExpectedDate {
				dateCorrect++
			} else {
				h.failures = append(h.failures, FailureRecord{
					ConfigName: configName,
					ItemID:     gt.ID,
					Title:      gt.Title,
					Expected:   fmt.Sprintf("Due Date: %s", gt.ExpectedDate),
					Got:        fmt.Sprintf("Due Date: %s", bestMatch.DueDate.Format("2006-01-02")),
					Reason:     "Date resolution discrepancy",
				})
			}

			// State accuracy check (cancellation, postponement, active)
			if bestMatch.Status == gt.ExpectedStatus {
				stateCorrect++
			} else {
				h.failures = append(h.failures, FailureRecord{
					ConfigName: configName,
					ItemID:     gt.ID,
					Title:      gt.Title,
					Expected:   fmt.Sprintf("Status: %s", gt.ExpectedStatus),
					Got:        fmt.Sprintf("Status: %s", bestMatch.Status),
					Reason:     "State transition discrepancy",
				})
			}

			if bestMatch.NeedsReview {
				reviewCount++
			}
		} else {
			fn++
			h.failures = append(h.failures, FailureRecord{
				ConfigName: configName,
				ItemID:     gt.ID,
				Title:      gt.Title,
				Expected:   "Event Detected",
				Got:        "Missed Event",
				Reason:     "Failed to detect deadline from message stream",
			})
		}
	}

	// Any unmatched detected items count as false positives
	for _, it := range items {
		if !matchedItems[it.ID] {
			fp++
		}
	}

	prec := 0.0
	if tp+fp > 0 {
		prec = float64(tp) / float64(tp+fp)
	}

	rec := 0.0
	if tp+fn > 0 {
		rec = float64(tp) / float64(tp+fn)
	}

	f1 := 0.0
	if prec+rec > 0 {
		f1 = 2 * (prec * rec) / (prec + rec)
	}

	dateAcc := 0.0
	stateAcc := 0.0
	if tp > 0 {
		dateAcc = float64(dateCorrect) / float64(tp)
		stateAcc = float64(stateCorrect) / float64(tp)
	}

	fpRate := 0.0
	if fp+tp > 0 {
		fpRate = float64(fp) / float64(fp+tp)
	}

	mode := "rule"
	if strings.Contains(strings.ToLower(configName), "gemma") {
		mode = "gemma"
	}

	return EvalResult{
		ConfigName:             configName,
		Mode:                   mode,
		TotalTrueEvents:        len(groundTruth),
		TruePositives:          tp,
		FalsePositives:         fp,
		FalseNegatives:         fn,
		Precision:              round2(prec),
		Recall:                 round2(rec),
		F1Score:                round2(f1),
		DateAccuracy:           round2(dateAcc),
		StateAccuracy:          round2(stateAcc),
		FalsePositiveNoiseRate: round2(fpRate),
		ItemsSentToReview:      reviewCount,
		SecondsElapsed:         round2(elapsedSec),
	}
}

// WriteReports outputs eval/RESULTS.md and eval/failures.md
func (h *Harness) WriteReports(resultsPath, failuresPath string) error {
	var sb strings.Builder
	sb.WriteString("# Aalsi Evaluation Benchmark Results\n\n")
	sb.WriteString("| Mode | Configuration | Precision | Recall | F1 | Date Accuracy | State Accuracy | FP Noise Rate | Review Queue | Seconds |\n")
	sb.WriteString("| :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	ranConfigs := make(map[string]EvalResult)
	for _, r := range h.results {
		ranConfigs[r.ConfigName] = r
		sb.WriteString(fmt.Sprintf("| `%s` | **%s** | %.2f | %.2f | %.2f | %.2f | %.2f | %.2f | %d | %.2fs |\n",
			r.Mode, r.ConfigName, r.Precision, r.Recall, r.F1Score, r.DateAccuracy, r.StateAccuracy,
			r.FalsePositiveNoiseRate, r.ItemsSentToReview, r.SecondsElapsed,
		))
	}

	// Always report separate rows for each expected mode and size; write TODO if a mode wasn't run
	requiredConfigs := []struct {
		Mode string
		Name string
	}{
		{"rule", "Rule Mode Baseline (No LLM)"},
		{"gemma", "Gemma 3 12B"},
		{"gemma", "Gemma 3 4B (Laptop Mode)"},
		{"gemma", "Gemma (No Overlap Ablation)"},
	}

	for _, req := range requiredConfigs {
		if _, ok := ranConfigs[req.Name]; !ok {
			sb.WriteString(fmt.Sprintf("| `%s` | **%s** | *TODO* | *TODO* | *TODO* | *TODO* | *TODO* | *TODO* | *TODO* | *TODO* |\n",
				req.Mode, req.Name,
			))
		}
	}

	sb.WriteString("\n*Benchmark generated deterministically from synthetic 3-week ground-truth chats. The harness strictly refuses to report Gemma results if Ollama is unreachable. Unrun modes are explicitly marked as TODO without merging or relabeling.*\n")

	if err := os.WriteFile(resultsPath, []byte(sb.String()), 0644); err != nil {
		return err
	}

	var fb strings.Builder
	fb.WriteString("# Detailed Mismatch & Failure Log\n\n")
	if len(h.failures) == 0 {
		fb.WriteString("No mismatches detected across evaluation runs.\n")
	} else {
		for _, f := range h.failures {
			fb.WriteString(fmt.Sprintf("### [%s] %s\n", f.ConfigName, f.Title))
			fb.WriteString(fmt.Sprintf("- **Item ID**: `%s`\n", f.ItemID))
			fb.WriteString(fmt.Sprintf("- **Expected**: %s\n", f.Expected))
			fb.WriteString(fmt.Sprintf("- **Got**: %s\n", f.Got))
			fb.WriteString(fmt.Sprintf("- **Reason**: %s\n\n", f.Reason))
		}
	}

	return os.WriteFile(failuresPath, []byte(fb.String()), 0644)
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
