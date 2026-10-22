package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aalsi-app/aalsi/pkg/store"
	"github.com/aalsi-app/aalsi/pkg/tracker"
)

func TestWatcher_SyncOnce(t *testing.T) {
	tempDir := t.TempDir()
	inboxDir := filepath.Join(tempDir, "inbox")
	outDir := filepath.Join(tempDir, "out")
	dbPath := filepath.Join(tempDir, "watcher.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer st.Close()

	tr := tracker.New(st)
	w := New(Config{
		InboxDir: inboxDir,
		OutDir:   outDir,
		Store:    st,
		Tracker:  tr,
	})

	_ = os.MkdirAll(inboxDir, 0755)
	_ = os.MkdirAll(outDir, 0755)

	// Drop chat export into inbox
	chatText := `12/10/25, 9:41 pm - Prof Sharma: Operating Systems Assignment 1 due 20th Oct.
12/10/25, 9:42 pm - Rahul: ok sir`

	err = os.WriteFile(filepath.Join(inboxDir, "cs101_chat.txt"), []byte(chatText), 0644)
	if err != nil {
		t.Fatalf("writing test chat file: %v", err)
	}

	imported, err := w.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}
	if imported != 2 {
		t.Errorf("expected 2 messages imported, got %d", imported)
	}

	// Verify out/ files generated
	digestFile := filepath.Join(outDir, "morning_digest.txt")
	calFile := filepath.Join(outDir, "aalsi_deadlines.ics")

	if _, err := os.Stat(digestFile); err != nil {
		t.Errorf("morning_digest.txt not found in outDir")
	}
	if _, err := os.Stat(calFile); err != nil {
		t.Errorf("aalsi_deadlines.ics not found in outDir")
	}
}
