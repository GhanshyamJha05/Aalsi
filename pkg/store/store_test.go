package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/parser"
)

func TestStore_IdempotentImport(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_aalsi.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	chatText := `12/10/25, 9:41 pm - Prof Sharma: Reminder: Operating Systems assignment 2 is due on 18th Oct.
12/10/25, 9:42 pm - Rahul: ok sir
12/10/25, 9:43 pm - Priya: <Media omitted>`

	parsed, err := parser.ParseChat(strings.NewReader(chatText), parser.ParseOptions{
		GroupID: "cs101",
	})
	if err != nil {
		t.Fatalf("ParseChat error: %v", err)
	}

	ctx := context.Background()

	// 1st Import: should insert 3 messages
	inserted1, err := db.ImportMessages(ctx, parsed.Messages)
	if err != nil {
		t.Fatalf("First ImportMessages failed: %v", err)
	}
	if inserted1 != 3 {
		t.Errorf("expected 3 inserted, got %d", inserted1)
	}

	// 2nd Import of identical export: must insert 0 (strictly idempotent)
	inserted2, err := db.ImportMessages(ctx, parsed.Messages)
	if err != nil {
		t.Fatalf("Second ImportMessages failed: %v", err)
	}
	if inserted2 != 0 {
		t.Errorf("expected 0 inserted on re-import, got %d", inserted2)
	}

	// Overlapping import with 1 new message
	chatTextOverlap := `12/10/25, 9:42 pm - Rahul: ok sir
12/10/25, 9:43 pm - Priya: <Media omitted>
12/10/25, 10:00 pm - Prof Sharma: Lab on Monday 10 AM.`

	parsedOverlap, err := parser.ParseChat(strings.NewReader(chatTextOverlap), parser.ParseOptions{
		GroupID: "cs101",
	})
	if err != nil {
		t.Fatalf("ParseChat overlap error: %v", err)
	}

	inserted3, err := db.ImportMessages(ctx, parsedOverlap.Messages)
	if err != nil {
		t.Fatalf("Overlapping ImportMessages failed: %v", err)
	}
	if inserted3 != 1 {
		t.Errorf("expected exactly 1 new message inserted from overlap, got %d", inserted3)
	}

	// Verify total stored count
	all, err := db.GetMessagesByGroup(ctx, "cs101")
	if err != nil {
		t.Fatalf("GetMessagesByGroup failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 total messages in group, got %d", len(all))
	}
}

func TestStore_DeadlineItemAndHistory(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_items.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now()
	due := now.Add(48 * time.Hour)

	item := &model.DeadlineItem{
		ID:              "item-123",
		GroupID:         "cs101",
		Kind:            model.KindAssignment,
		Title:           "OS Lab 2",
		SubjectOrCourse: "Operating Systems",
		DueDate:         due,
		HasExplicitTime: true,
		Status:          model.StatusActive,
		Confidence:      0.95,
		NeedsReview:     false,
		ActionNeeded:    "Submit report on portal",
		CreatedAt:       now,
		UpdatedAt:       now,
		Evidence: []model.ItemHistory{
			{
				ID:          "hist-1",
				ItemID:      "item-123",
				MessageID:   "msg-001",
				Sender:      "Prof Sharma",
				Timestamp:   now,
				MessageText: "OS Lab 2 due in 2 days",
				ActionTaken: "created",
				NewDueDate:  &due,
				RecordedAt:  now,
			},
		},
	}

	if err := db.SaveDeadlineItem(ctx, item); err != nil {
		t.Fatalf("SaveDeadlineItem failed: %v", err)
	}

	items, err := db.GetItems(ctx, false)
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	fetched := items[0]
	if fetched.Title != "OS Lab 2" || fetched.SubjectOrCourse != "Operating Systems" {
		t.Errorf("unexpected item fields: %+v", fetched)
	}
	if len(fetched.Evidence) != 1 {
		t.Errorf("expected 1 evidence entry, got %d", len(fetched.Evidence))
	}

	// Purge test
	if err := db.PurgeAll(ctx); err != nil {
		t.Fatalf("PurgeAll failed: %v", err)
	}
	itemsAfterPurge, err := db.GetItems(ctx, false)
	if err != nil {
		t.Fatalf("GetItems after purge failed: %v", err)
	}
	if len(itemsAfterPurge) != 0 {
		t.Errorf("expected 0 items after purge, got %d", len(itemsAfterPurge))
	}
}
