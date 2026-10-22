package tracker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/store"
)

func ptrString(s string) *string {
	return &s
}

func setupTestStore(t *testing.T) (*store.Store, *Engine) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "tracker_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	engine := New(st)
	return st, engine
}

func TestTracker_Scenarios(t *testing.T) {
	st, engine := setupTestStore(t)
	defer st.Close()
	ctx := context.Background()

	loc := time.FixedZone("IST", 5*3600+30*60)
	baseTime := time.Date(2025, 10, 15, 10, 0, 0, 0, loc)

	// Scenario 1: Initial Item Created ("OS Assignment 1 due 20th Oct")
	msgMap := map[string]model.Message{
		"msg-1": {
			ID:        "msg-1",
			Sender:    "Prof Sharma",
			Timestamp: baseTime,
			Text:      "Operating Systems Assignment 1 is due on 20th Oct 5 PM.",
		},
	}
	raw1 := model.RawExtractedEvent{
		Kind:              model.KindAssignment,
		Title:             "OS Assignment 1",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "20th Oct",
		RawTimeExpression: ptrString("5 PM"),
		SourceMessageIDs:  []string{"msg-1"},
		Confidence:        0.95,
	}

	item1, err := engine.ProcessRawEvent(ctx, "cs101", raw1, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent 1 failed: %v", err)
	}
	if item1.Status != model.StatusActive {
		t.Errorf("expected StatusActive, got %s", item1.Status)
	}
	if len(item1.Evidence) != 1 {
		t.Errorf("expected 1 evidence, got %d", len(item1.Evidence))
	}

	// Scenario 2: Duplicate Reminder ("Reminder: OS Assignment 1 due 20th Oct")
	// MUST MERGE into same item without creating a new item
	msgMap["msg-2"] = model.Message{
		ID:        "msg-2",
		Sender:    "CR Amit",
		Timestamp: baseTime.Add(24 * time.Hour),
		Text:      "Reminder: OS Assignment 1 due on 20th Oct 5 PM.",
	}
	rawReminder := model.RawExtractedEvent{
		Kind:              model.KindAssignment,
		Title:             "OS Assignment 1",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "20th Oct",
		RawTimeExpression: ptrString("5 PM"),
		SourceMessageIDs:  []string{"msg-2"},
		Confidence:        0.90,
	}

	itemMerged, err := engine.ProcessRawEvent(ctx, "cs101", rawReminder, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent reminder failed: %v", err)
	}
	if itemMerged.ID != item1.ID {
		t.Errorf("expected same item ID %s, got %s", item1.ID, itemMerged.ID)
	}
	if len(itemMerged.Evidence) != 2 {
		t.Errorf("expected 2 evidence entries after reminder merge, got %d", len(itemMerged.Evidence))
	}
	if itemMerged.Evidence[1].ActionTaken != "reminder" {
		t.Errorf("expected action 'reminder', got %s", itemMerged.Evidence[1].ActionTaken)
	}

	// Scenario 3: Extension ("OS Assignment 1 extended by 2 days to 22nd Oct")
	msgMap["msg-3"] = model.Message{
		ID:        "msg-3",
		Sender:    "Prof Sharma",
		Timestamp: baseTime.Add(48 * time.Hour),
		Text:      "OS Assignment 1 deadline extended to 22nd Oct 5 PM.",
	}
	rawExtension := model.RawExtractedEvent{
		Kind:              model.KindPostponement,
		Title:             "OS Assignment 1",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "22nd Oct",
		RawTimeExpression: ptrString("5 PM"),
		RefersToEarlier:   ptrString("OS Assignment 1"),
		SourceMessageIDs:  []string{"msg-3"},
		Confidence:        0.95,
	}

	itemExtended, err := engine.ProcessRawEvent(ctx, "cs101", rawExtension, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent extension failed: %v", err)
	}
	if itemExtended.ID != item1.ID {
		t.Errorf("expected item ID to be preserved across extension")
	}
	if itemExtended.Status != model.StatusPostponed {
		t.Errorf("expected StatusPostponed, got %s", itemExtended.Status)
	}
	if itemExtended.DueDate.Day() != 22 {
		t.Errorf("expected DueDate day 22, got %d", itemExtended.DueDate.Day())
	}

	// Scenario 4: Different assignment with similar name ("OS Assignment 2 due 28th Oct")
	// MUST NOT merge with Assignment 1!
	msgMap["msg-4"] = model.Message{
		ID:        "msg-4",
		Sender:    "Prof Sharma",
		Timestamp: baseTime.Add(72 * time.Hour),
		Text:      "OS Assignment 2 is released, due 28th Oct.",
	}
	rawAssignment2 := model.RawExtractedEvent{
		Kind:              model.KindAssignment,
		Title:             "OS Assignment 2",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "28th Oct",
		SourceMessageIDs:  []string{"msg-4"},
		Confidence:        0.95,
	}

	item2, err := engine.ProcessRawEvent(ctx, "cs101", rawAssignment2, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent assignment 2 failed: %v", err)
	}
	if item2.ID == item1.ID {
		t.Fatalf("CRITICAL ERROR: OS Assignment 2 merged into OS Assignment 1!")
	}
	if item2.DueDate.Day() != 28 {
		t.Errorf("expected DueDate 28, got %d", item2.DueDate.Day())
	}

	// Scenario 5: Cancellation ("OS Assignment 2 cancelled")
	msgMap["msg-5"] = model.Message{
		ID:        "msg-5",
		Sender:    "Prof Sharma",
		Timestamp: baseTime.Add(96 * time.Hour),
		Text:      "Attention: OS Assignment 2 is cancelled due to tech festival.",
	}
	rawCancel := model.RawExtractedEvent{
		Kind:              model.KindCancellation,
		Title:             "OS Assignment 2",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "28th Oct",
		RefersToEarlier:   ptrString("OS Assignment 2"),
		SourceMessageIDs:  []string{"msg-5"},
		Confidence:        0.95,
	}

	itemCancelled, err := engine.ProcessRawEvent(ctx, "cs101", rawCancel, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent cancellation failed: %v", err)
	}
	if itemCancelled.ID != item2.ID {
		t.Errorf("expected cancelled item to match Assignment 2 ID")
	}
	if itemCancelled.Status != model.StatusCancelled {
		t.Errorf("expected StatusCancelled, got %s", itemCancelled.Status)
	}

	// Scenario 6: Correction changing time only ("OS Assignment 1 time shifted to 11:59 PM")
	msgMap["msg-6"] = model.Message{
		ID:        "msg-6",
		Sender:    "Prof Sharma",
		Timestamp: baseTime.Add(100 * time.Hour),
		Text:      "OS Assignment 1 submission time shifted to 11:59 PM tonight (22nd Oct).",
	}
	rawTimeCorrection := model.RawExtractedEvent{
		Kind:              model.KindPostponement,
		Title:             "OS Assignment 1",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "22nd Oct",
		RawTimeExpression: ptrString("11:59 PM"),
		RefersToEarlier:   ptrString("OS Assignment 1"),
		SourceMessageIDs:  []string{"msg-6"},
		Confidence:        0.95,
	}

	itemTimeCorrected, err := engine.ProcessRawEvent(ctx, "cs101", rawTimeCorrection, msgMap)
	if err != nil {
		t.Fatalf("ProcessRawEvent time correction failed: %v", err)
	}
	if itemTimeCorrected.ID != item1.ID {
		t.Errorf("expected item 1 ID")
	}
	if itemTimeCorrected.DueDate.Hour() != 23 || itemTimeCorrected.DueDate.Minute() != 59 {
		t.Errorf("expected 23:59, got %02d:%02d", itemTimeCorrected.DueDate.Hour(), itemTimeCorrected.DueDate.Minute())
	}

	// Verify total distinct items in database: exactly 2 items (Assignment 1 and Assignment 2)
	allItems, err := st.GetItems(ctx, true)
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(allItems) != 2 {
		t.Fatalf("expected exactly 2 items in store, got %d", len(allItems))
	}
}
