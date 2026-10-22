package digest

import (
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

func TestDigest_GroupingAndLanguages(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2025, 10, 15, 8, 0, 0, 0, loc)

	items := []model.DeadlineItem{
		{
			ID:              "item-past",
			Title:           "Fee Submission",
			DueDate:         now.AddDate(0, 0, -2), // overdue
			Status:          model.StatusActive,
			NeedsReview:     false,
			SubjectOrCourse: "Admin",
		},
		{
			ID:              "item-today",
			Title:           "OS Lab 1",
			DueDate:         now.Add(6 * time.Hour), // today 2 PM
			HasExplicitTime: true,
			Status:          model.StatusActive,
			NeedsReview:     false,
			SubjectOrCourse: "Operating Systems",
		},
		{
			ID:              "item-tomorrow",
			Title:           "Maths Quiz",
			DueDate:         now.AddDate(0, 0, 1), // tomorrow
			Status:          model.StatusActive,
			NeedsReview:     false,
			SubjectOrCourse: "Mathematics",
		},
		{
			ID:              "item-this-week",
			Title:           "DBMS Mini Project",
			DueDate:         now.AddDate(0, 0, 4), // 4 days later
			Status:          model.StatusActive,
			NeedsReview:     false,
			SubjectOrCourse: "DBMS",
		},
		{
			ID:              "item-review",
			Title:           "Ambiguous Lab",
			DueDate:         now.AddDate(0, 0, 1),
			Status:          model.StatusActive,
			NeedsReview:     true, // Review queue
		},
	}

	// 1. English
	dEng := Generate(items, now, LangEnglish)
	if len(dEng.Today) != 1 || dEng.Today[0].ID != "item-today" {
		t.Errorf("expected 1 today item")
	}
	if len(dEng.Tomorrow) != 1 || dEng.Tomorrow[0].ID != "item-tomorrow" {
		t.Errorf("expected 1 tomorrow item")
	}
	if len(dEng.ThisWeek) != 1 || dEng.ThisWeek[0].ID != "item-this-week" {
		t.Errorf("expected 1 this-week item")
	}
	if len(dEng.YouMayHaveMissed) != 1 || dEng.YouMayHaveMissed[0].ID != "item-past" {
		t.Errorf("expected 1 missed item")
	}
	if len(dEng.ReviewQueue) != 1 || dEng.ReviewQueue[0].ID != "item-review" {
		t.Errorf("expected 1 review item")
	}
	if !strings.Contains(dEng.Summary, "1 deadline due today") {
		t.Errorf("unexpected summary: %s", dEng.Summary)
	}

	// Plain text format
	txt := dEng.FormatPlainText()
	if !strings.Contains(txt, "Aalsi Morning Digest") || !strings.Contains(txt, "Aalsi did the reading") {
		t.Errorf("plain text missing microcopy: %s", txt)
	}

	// 2. Hindi
	dHi := Generate(items, now, LangHindi)
	if !strings.Contains(dHi.Summary, "आज आपके लिए 1 काम निर्धारित है") {
		t.Errorf("unexpected Hindi summary: %s", dHi.Summary)
	}

	// 3. Hinglish
	dHing := Generate(items, now, LangHinglish)
	if !strings.Contains(dHing.Summary, "Aaj ke liye 1 deadline hai") {
		t.Errorf("unexpected Hinglish summary: %s", dHing.Summary)
	}
}
