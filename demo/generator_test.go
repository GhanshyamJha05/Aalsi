package demo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/parser"
)

func TestGenerateDemoData(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now()

	err := GenerateDemoData(tempDir, now)
	if err != nil {
		t.Fatalf("GenerateDemoData failed: %v", err)
	}

	// Verify files exist
	androidFile := filepath.Join(tempDir, "college_chat_android.txt")
	iosFile := filepath.Join(tempDir, "college_chat_ios.txt")
	gtFile := filepath.Join(tempDir, "ground_truth.json")

	for _, f := range []string{androidFile, iosFile, gtFile} {
		info, err := os.Stat(f)
		if err != nil || info.Size() == 0 {
			t.Fatalf("File %s does not exist or is empty", f)
		}
	}

	// Verify parser can parse generated android export
	data, err := os.ReadFile(androidFile)
	if err != nil {
		t.Fatalf("reading android file: %v", err)
	}
	res, err := parser.ParseChat(strings.NewReader(string(data)), parser.ParseOptions{})
	if err != nil {
		t.Fatalf("ParseChat failed on generated demo: %v", err)
	}

	if len(res.Messages) < 15 {
		t.Errorf("expected at least 15 messages, got %d", len(res.Messages))
	}
}
