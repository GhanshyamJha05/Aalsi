package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/digest"
	"github.com/aalsi-app/aalsi/pkg/extractor"
	"github.com/aalsi-app/aalsi/pkg/ics"
	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/parser"
	"github.com/aalsi-app/aalsi/pkg/prefilter"
	"github.com/aalsi-app/aalsi/pkg/store"
	"github.com/aalsi-app/aalsi/pkg/tracker"
)

// Watcher monitors an inbox folder for exported chats and generates updated outputs
type Watcher struct {
	inboxDir  string
	outDir    string
	store     *store.Store
	tracker   *tracker.Engine
	extractor *extractor.Extractor
	loc       *time.Location
	pollFreq  time.Duration
}

// Config configures the Watcher
type Config struct {
	InboxDir  string
	OutDir    string
	Store     *store.Store
	Tracker   *tracker.Engine
	Extractor *extractor.Extractor
	Location  *time.Location
	PollFreq  time.Duration
}

// New creates a directory Watcher instance
func New(cfg Config) *Watcher {
	if cfg.Location == nil {
		cfg.Location = time.FixedZone("IST", 5*3600+30*60)
	}
	if cfg.PollFreq == 0 {
		cfg.PollFreq = 5 * time.Second
	}
	return &Watcher{
		inboxDir:  cfg.InboxDir,
		outDir:    cfg.OutDir,
		store:     cfg.Store,
		tracker:   cfg.Tracker,
		extractor: cfg.Extractor,
		loc:       cfg.Location,
		pollFreq:  cfg.PollFreq,
	}
}

// Start runs the watcher loop until ctx is cancelled
func (w *Watcher) Start(ctx context.Context) error {
	if err := os.MkdirAll(w.inboxDir, 0755); err != nil {
		return fmt.Errorf("creating inbox dir: %w", err)
	}
	if err := os.MkdirAll(w.outDir, 0755); err != nil {
		return fmt.Errorf("creating out dir: %w", err)
	}

	log.Printf("[LAZY MODE] Watching %s for new WhatsApp exports. Outputs will be written to %s", w.inboxDir, w.outDir)

	// Run initial sync
	w.SyncOnce(ctx)

	ticker := time.NewTicker(w.pollFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.SyncOnce(ctx)
		}
	}
}

// SyncOnce scans inbox, processes unimported files, and regenerates out files
func (w *Watcher) SyncOnce(ctx context.Context) (int, error) {
	files, err := os.ReadDir(w.inboxDir)
	if err != nil {
		return 0, err
	}

	totalImported := 0

	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			continue
		}

		filePath := filepath.Join(w.inboxDir, entry.Name())
		f, err := os.Open(filePath)
		if err != nil {
			continue
		}

		groupID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		parsed, err := parser.ParseChat(f, parser.ParseOptions{
			GroupID:  groupID,
			Location: w.loc,
		})
		f.Close()
		if err != nil {
			continue
		}

		inserted, err := w.store.ImportMessages(ctx, parsed.Messages)
		if err != nil || inserted == 0 {
			// File already imported or no new messages
			continue
		}

		totalImported += inserted
		log.Printf("[LAZY MODE] Imported %d new messages from %s", inserted, entry.Name())

		// Process through prefilter & Gemma
		candidates := prefilter.FilterMessages(parsed.Messages)
		msgMap := make(map[string]model.Message)
		for _, m := range parsed.Messages {
			msgMap[m.ID] = m
		}

		if w.extractor != nil && len(candidates) > 0 {
			events, err := w.extractor.ChunkAndExtract(ctx, candidates, 3500, 500)
			if err != nil {
				log.Printf("[LAZY WATCHER] Ollama offline (%v). Using deterministic fallback extractor.", err)
				events = extractor.FallbackRuleExtractor(candidates)
			}
			for _, ev := range events {
				w.tracker.ProcessRawEvent(ctx, groupID, ev, msgMap)
			}
		}
	}

	// Always ensure out/ files are up-to-date
	items, err := w.store.GetItems(ctx, false)
	if err == nil {
		d := digest.Generate(items, time.Now().In(w.loc), digest.LangEnglish)
		_ = os.WriteFile(filepath.Join(w.outDir, "morning_digest.txt"), []byte(d.FormatPlainText()), 0644)

		cal := ics.GenerateCalendar(items)
		_ = os.WriteFile(filepath.Join(w.outDir, "aalsi_deadlines.ics"), []byte(cal), 0644)
	}

	return totalImported, nil
}
