package tracker

import (
	"context"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

func TestTracker_TableDrivenScenarios(t *testing.T) {
	st, engine := setupTestStore(t)
	defer st.Close()
	ctx := context.Background()

	loc := time.FixedZone("IST", 5*3600+30*60)
	baseTime := time.Date(2025, 10, 15, 10, 0, 0, 0, loc)

	// Step 1: Create initial items:
	// - Maths Quiz (KindExam)
	// - OS Assignment 1 (KindAssignment)
	// - Physics Class Lab (KindEvent)
	msgMap := map[string]model.Message{
		"m-quiz": {
			ID:        "m-quiz",
			Sender:    "Prof Verma",
			Timestamp: baseTime,
			Text:      "Maths surprise quiz on Friday 10 AM.",
		},
		"m-asg1": {
			ID:        "m-asg1",
			Sender:    "Prof Sharma",
			Timestamp: baseTime,
			Text:      "Operating Systems Assignment 1 due 20th Oct 5 PM.",
		},
		"m-class": {
			ID:        "m-class",
			Sender:    "Prof Roy",
			Timestamp: baseTime,
			Text:      "Physics lab class tomorrow 3 PM.",
		},
	}

	quizItem, err := engine.ProcessRawEvent(ctx, "secA", model.RawExtractedEvent{
		Kind:              model.KindExam,
		Title:             "Mathematics Quiz",
		SubjectOrCourse:   ptrString("Mathematics"),
		RawDateExpression: "Friday",
		RawTimeExpression: ptrString("10 AM"),
		SourceMessageIDs:  []string{"m-quiz"},
		Confidence:        0.95,
	}, msgMap)
	if err != nil {
		t.Fatalf("Creating quiz item failed: %v", err)
	}

	asg1Item, err := engine.ProcessRawEvent(ctx, "secA", model.RawExtractedEvent{
		Kind:              model.KindAssignment,
		Title:             "OS Assignment 1",
		SubjectOrCourse:   ptrString("Operating Systems"),
		RawDateExpression: "20th Oct",
		RawTimeExpression: ptrString("5 PM"),
		SourceMessageIDs:  []string{"m-asg1"},
		Confidence:        0.95,
	}, msgMap)
	if err != nil {
		t.Fatalf("Creating asg1 item failed: %v", err)
	}

	classItem, err := engine.ProcessRawEvent(ctx, "secA", model.RawExtractedEvent{
		Kind:              model.KindEvent,
		Title:             "Physics Lab Class",
		SubjectOrCourse:   ptrString("Physics"),
		RawDateExpression: "tomorrow",
		RawTimeExpression: ptrString("3 PM"),
		SourceMessageIDs:  []string{"m-class"},
		Confidence:        0.95,
	}, msgMap)
	if err != nil {
		t.Fatalf("Creating class item failed: %v", err)
	}

	// Table-driven test cases required by Requirement 2 & 3:
	testCases := []struct {
		name                 string
		rawEvent             model.RawExtractedEvent
		msg                  model.Message
		expectedStatus       model.ItemStatus
		expectedTargetItemID string // empty if new item
		mustNotMatchItemID   string
		expectedNeedsReview  bool
		verifyTitleNotKind   bool
	}{
		{
			name: "Case 1: Quiz postponed - updates existing item with postponed status and new date",
			msg: model.Message{
				ID:        "m-quiz-postpone",
				Sender:    "Prof Verma",
				Timestamp: baseTime.Add(24 * time.Hour),
				Text:      "Maths quiz is postponed by 2 days to Sunday 10 AM.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindPostponement,
				Title:             "Mathematics Quiz",
				SubjectOrCourse:   ptrString("Mathematics"),
				RawDateExpression: "Sunday",
				RawTimeExpression: ptrString("10 AM"),
				RefersToEarlier:   ptrString("Mathematics Quiz"),
				Confidence:        0.95,
			},
			expectedStatus:       model.StatusPostponed,
			expectedTargetItemID: quizItem.ID,
		},
		{
			name: "Case 2: Assignment extended by 2 days - updates existing item",
			msg: model.Message{
				ID:        "m-asg1-ext",
				Sender:    "Prof Sharma",
				Timestamp: baseTime.Add(48 * time.Hour),
				Text:      "OS Assignment 1 deadline extended to 22nd Oct 5 PM.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindPostponement,
				Title:             "OS Assignment 1",
				SubjectOrCourse:   ptrString("Operating Systems"),
				RawDateExpression: "22nd Oct",
				RawTimeExpression: ptrString("5 PM"),
				RefersToEarlier:   ptrString("OS Assignment 1"),
				Confidence:        0.95,
			},
			expectedStatus:       model.StatusPostponed,
			expectedTargetItemID: asg1Item.ID,
		},
		{
			name: "Case 3: Class cancelled - marks existing item cancelled",
			msg: model.Message{
				ID:        "m-class-cancel",
				Sender:    "Prof Roy",
				Timestamp: baseTime.Add(12 * time.Hour),
				Text:      "Physics lab class is cancelled for tomorrow.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindCancellation,
				Title:             "Physics Lab Class",
				SubjectOrCourse:   ptrString("Physics"),
				RawDateExpression: "tomorrow",
				RefersToEarlier:   ptrString("Physics Lab Class"),
				Confidence:        0.95,
			},
			expectedStatus:       model.StatusCancelled,
			expectedTargetItemID: classItem.ID,
		},
		{
			name: "Case 4: Two similarly named assignments must NOT merge (OS Assignment 2)",
			msg: model.Message{
				ID:        "m-asg2",
				Sender:    "Prof Sharma",
				Timestamp: baseTime.Add(72 * time.Hour),
				Text:      "OS Assignment 2 released, due 28th Oct 5 PM.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindAssignment,
				Title:             "OS Assignment 2",
				SubjectOrCourse:   ptrString("Operating Systems"),
				RawDateExpression: "28th Oct",
				RawTimeExpression: ptrString("5 PM"),
				Confidence:        0.95,
			},
			expectedStatus:     model.StatusActive,
			mustNotMatchItemID: asg1Item.ID,
		},
		{
			name: "Case 5: Unmatched cancellation must NEVER have title set to 'cancellation'",
			msg: model.Message{
				ID:        "m-chem-cancel",
				Sender:    "Prof Das",
				Timestamp: baseTime,
				Text:      "Chemistry seminar cancelled today.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindCancellation,
				Title:             "cancellation", // Raw LLM artifact
				SubjectOrCourse:   ptrString("Chemistry"),
				RawDateExpression: "today",
				Confidence:        0.90,
			},
			expectedStatus:     model.StatusCancelled,
			verifyTitleNotKind: true,
		},
		{
			name: "Case 6: Requirement 3 - Exam without explicit time must go to Review Queue",
			msg: model.Message{
				ID:        "m-exam-eod",
				Sender:    "Teacher",
				Timestamp: baseTime,
				Text:      "Maths final exam on Monday.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindExam,
				Title:             "Maths Final Exam",
				SubjectOrCourse:   ptrString("Mathematics"),
				RawDateExpression: "Monday",
				// No time expression
				Confidence: 0.95,
			},
			expectedNeedsReview: true,
		},
		{
			name: "Case 7: Requirement 4 - Assignment without subject goes to Review Queue",
			msg: model.Message{
				ID:        "m-nosubject",
				Sender:    "Anonymous",
				Timestamp: baseTime,
				Text:      "Assignment submission due tomorrow EOD.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindAssignment,
				Title:             "Assignment 3",
				SubjectOrCourse:   nil, // No subject!
				RawDateExpression: "tomorrow",
				Confidence:        0.85,
			},
			expectedNeedsReview: true,
		},
		{
			name: "Case 8: Requirement 3 - Confidence below 0.7 goes to Review Queue",
			msg: model.Message{
				ID:        "m-lowconf",
				Sender:    "Student",
				Timestamp: baseTime,
				Text:      "I heard OS lab might be on Tuesday maybe.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindNotice,
				Title:             "OS Lab Rumor",
				SubjectOrCourse:   ptrString("Operating Systems"),
				RawDateExpression: "Tuesday",
				Confidence:        0.65, // Below 0.70!
			},
			expectedNeedsReview: true,
		},
		{
			name: "Case 9: Requirement 4 - Fee without subject does NOT go to Review Queue",
			msg: model.Message{
				ID:        "m-fee-nosubj",
				Sender:    "Accounts Dept",
				Timestamp: baseTime,
				Text:      "Notice: Semester fee payment due tomorrow.",
			},
			rawEvent: model.RawExtractedEvent{
				Kind:              model.KindFee,
				Title:             "Semester Exam Fee",
				SubjectOrCourse:   nil, // Fees do not require a subject!
				RawDateExpression: "tomorrow",
				Confidence:        0.90,
			},
			expectedNeedsReview: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.rawEvent.SourceMessageIDs = []string{tc.msg.ID}
			msgMap[tc.msg.ID] = tc.msg

			item, err := engine.ProcessRawEvent(ctx, "secA", tc.rawEvent, msgMap)
			if err != nil {
				t.Fatalf("ProcessRawEvent failed: %v", err)
			}

			if tc.expectedTargetItemID != "" && item.ID != tc.expectedTargetItemID {
				t.Errorf("Expected to match existing item ID %s, but got %s", tc.expectedTargetItemID, item.ID)
			}

			if tc.mustNotMatchItemID != "" && item.ID == tc.mustNotMatchItemID {
				t.Errorf("Item must NOT match ID %s, but matched!", tc.mustNotMatchItemID)
			}

			if tc.expectedStatus != "" && item.Status != tc.expectedStatus {
				t.Errorf("Expected status %s, got %s", tc.expectedStatus, item.Status)
			}

			if tc.verifyTitleNotKind && (item.Title == "cancellation" || item.Title == "postponement") {
				t.Errorf("Title must not be kind string, got %s", item.Title)
			}

			if tc.expectedNeedsReview && !item.NeedsReview {
				t.Errorf("Item was expected to have NeedsReview=true, got false")
			}
		})
	}
}
