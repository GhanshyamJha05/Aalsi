package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseChat_Android(t *testing.T) {
	input := `12/10/25, 9:41 pm - Messages and calls are end-to-end encrypted.
12/10/25, 9:41 pm - Prof Sharma: Reminder: Operating Systems assignment 2 is due on 18th Oct.
Submit by 5 PM sharp.
12/10/25, 9:42 pm - Rahul: ok sir
12/10/25, 9:43 pm - Priya: <Media omitted>
12/10/25, 9:45 pm - Rahul: This message was deleted`

	res, err := ParseChat(strings.NewReader(input), ParseOptions{
		GroupID: "cs101",
	})
	if err != nil {
		t.Fatalf("ParseChat error: %v", err)
	}

	if len(res.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(res.Messages))
	}

	// 1st is system
	if !res.Messages[0].IsSystem {
		t.Errorf("expected 1st message to be system notification")
	}

	// 2nd is multi-line from Prof Sharma
	m2 := res.Messages[1]
	if m2.Sender != "Prof Sharma" {
		t.Errorf("expected sender Prof Sharma, got %s", m2.Sender)
	}
	if !strings.Contains(m2.Text, "Submit by 5 PM sharp.") {
		t.Errorf("expected multiline text to be collated, got %s", m2.Text)
	}
	if m2.Timestamp.Hour() != 21 || m2.Timestamp.Minute() != 41 {
		t.Errorf("expected 21:41 (9:41 PM), got %02d:%02d", m2.Timestamp.Hour(), m2.Timestamp.Minute())
	}
	if m2.GroupID != "cs101" {
		t.Errorf("expected GroupID cs101, got %s", m2.GroupID)
	}

	// 4th is media omitted
	if !res.Messages[3].IsMedia {
		t.Errorf("expected 4th message to be marked IsMedia")
	}

	// 5th is deleted message
	if !res.Messages[4].IsDeleted {
		t.Errorf("expected 5th message to be marked IsDeleted")
	}
}

func TestParseChat_iOS(t *testing.T) {
	input := `[15/10/25, 10:15:30 AM] CR Amit: Lab viva shifted to tomorrow 3 PM.
[15/10/25, 10:16:00 AM] Sneha: noted!
[15/10/25, 2:30:15 PM] CR Amit: Fee payment portal link active. Last date 28th Oct.`

	res, err := ParseChat(strings.NewReader(input), ParseOptions{
		GroupID: "batch2026",
	})
	if err != nil {
		t.Fatalf("ParseChat error: %v", err)
	}

	if len(res.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(res.Messages))
	}

	if res.Messages[0].Sender != "CR Amit" {
		t.Errorf("expected sender CR Amit, got %s", res.Messages[0].Sender)
	}
	if res.Messages[0].Timestamp.Hour() != 10 || res.Messages[0].Timestamp.Minute() != 15 {
		t.Errorf("expected 10:15 AM, got %02d:%02d", res.Messages[0].Timestamp.Hour(), res.Messages[0].Timestamp.Minute())
	}
	if res.Messages[2].Timestamp.Hour() != 14 || res.Messages[2].Timestamp.Minute() != 30 {
		t.Errorf("expected 14:30 (2:30 PM), got %02d:%02d", res.Messages[2].Timestamp.Hour(), res.Messages[2].Timestamp.Minute())
	}
}

func TestParseChat_DateFormatAmbiguity(t *testing.T) {
	// Day > 12 in first position -> DD/MM
	inputDMY := `25/08/25, 10:00 am - Teacher: Welcome
05/09/25, 11:00 am - Teacher: Assignment 1 due`

	resDMY, err := ParseChat(strings.NewReader(inputDMY), ParseOptions{})
	if err != nil {
		t.Fatalf("ParseChat error: %v", err)
	}
	if resDMY.InferredFormat != "DD/MM/YY" {
		t.Errorf("expected DD/MM/YY inferred, got %s", resDMY.InferredFormat)
	}
	if resDMY.AmbiguousWarning != "" {
		t.Errorf("expected no ambiguous warning when day > 12, got %s", resDMY.AmbiguousWarning)
	}

	// Ambiguous case (both <= 12)
	inputAmbig := `04/05/25, 10:00 am - Teacher: Welcome`
	var warned string
	resAmbig, err := ParseChat(strings.NewReader(inputAmbig), ParseOptions{
		WarnHandler: func(w string) { warned = w },
	})
	if err != nil {
		t.Fatalf("ParseChat error: %v", err)
	}
	if resAmbig.InferredFormat != "DD/MM/YY" {
		t.Errorf("expected default DD/MM/YY, got %s", resAmbig.InferredFormat)
	}
	if warned == "" || resAmbig.AmbiguousWarning == "" {
		t.Errorf("expected ambiguity warning to be triggered")
	}
}

func TestComputeMessageID_Stability(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	tm := time.Date(2025, 10, 12, 14, 30, 0, 0, loc)

	id1 := ComputeMessageID(tm, "Prof Sharma", "Assignment due Friday")
	id2 := ComputeMessageID(tm, "Prof Sharma", "  Assignment due Friday  ")
	id3 := ComputeMessageID(tm, "Prof Sharma", "Assignment due Saturday")

	if id1 == "" {
		t.Fatalf("expected non-empty message ID")
	}
	if id1 != id2 {
		t.Errorf("expected whitespace normalization to yield identical ID, got %s vs %s", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("expected different content to yield different ID")
	}
}
