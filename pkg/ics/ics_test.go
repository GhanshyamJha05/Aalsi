package ics

import (
	"strings"
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

func TestGenerateCalendar(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	due := time.Date(2025, 10, 20, 17, 0, 0, 0, loc)

	items := []model.DeadlineItem{
		{
			ID:              "item-1",
			Title:           "OS Assignment 1",
			SubjectOrCourse: "Operating Systems",
			DueDate:         due,
			Status:          model.StatusActive,
			NeedsReview:     false,
			ActionNeeded:    "Submit PDF",
			Evidence: []model.ItemHistory{
				{
					Sender:      "Prof Sharma",
					Timestamp:   due.AddDate(0, 0, -3),
					MessageText: "OS Assignment 1 due 20th Oct 5 PM",
				},
			},
		},
		{
			ID:          "item-cancelled",
			Title:       "Cancelled Quiz",
			DueDate:     due,
			Status:      model.StatusCancelled, // Must be omitted
			NeedsReview: false,
		},
		{
			ID:          "item-review",
			Title:       "Low Conf Item",
			DueDate:     due,
			Status:      model.StatusActive,
			NeedsReview: true, // Must be omitted from ICS until confirmed
		},
	}

	cal := GenerateCalendar(items)

	if !strings.HasPrefix(cal, "BEGIN:VCALENDAR") {
		t.Errorf("expected BEGIN:VCALENDAR header")
	}
	if !strings.Contains(cal, "SUMMARY:[Operating Systems] OS Assignment 1") {
		t.Errorf("expected course prefix in summary")
	}
	if !strings.Contains(cal, "TRIGGER:-P1D") || !strings.Contains(cal, "TRIGGER:-PT1H") {
		t.Errorf("expected 1-day and 1-hour VALARM triggers")
	}
	if !strings.Contains(cal, "Prof Sharma") || !strings.Contains(cal, "Evidence from WhatsApp") {
		t.Errorf("expected evidence in description")
	}
	if strings.Contains(cal, "Cancelled Quiz") {
		t.Errorf("cancelled items must not appear in calendar")
	}
	if strings.Contains(cal, "Low Conf Item") {
		t.Errorf("items needing review must not appear in calendar until confirmed")
	}
}
