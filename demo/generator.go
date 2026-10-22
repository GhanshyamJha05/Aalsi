package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// GroundTruthItem represents an expected event for evaluation
type GroundTruthItem struct {
	ID              string          `json:"id"`
	Kind            model.EventKind `json:"kind"`
	Title           string          `json:"title"`
	Subject         string          `json:"subject"`
	ExpectedDate    string          `json:"expected_date"` // YYYY-MM-DD
	ExpectedTime    string          `json:"expected_time"` // HH:MM or ""
	ExpectedStatus  model.ItemStatus `json:"expected_status"`
	EvidenceSnippet string          `json:"evidence_snippet"`
	IsTrap          bool            `json:"is_trap"` // true for noise/questions/cancellations
}

// GenerateDemoData produces a 3-week synthetic college group chat in Android and iOS formats
// relative to now, accompanied by a ground_truth.json
func GenerateDemoData(outDir string, now time.Time) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	// Anchor: 14 days ago to 7 days ahead
	d14Ago := now.AddDate(0, 0, -14)

	// Format helpers
	andFormat := func(t time.Time, sender, text string) string {
		return fmt.Sprintf("%s - %s: %s", t.Format("02/01/06, 3:04 pm"), sender, text)
	}
	iosFormat := func(t time.Time, sender, text string) string {
		return fmt.Sprintf("[%s] %s: %s", t.Format("02/01/06, 3:04:05 PM"), sender, text)
	}

	type chatLine struct {
		t      time.Time
		sender string
		text   string
	}

	// 15 True Deadlines across 4 courses:
	// Courses: Operating Systems (OS), Database Systems (DBMS), Mathematics (Maths), Computer Networks (CN)
	// Traps:
	// - Relative dates: kal, parso, Friday
	// - 2 postponements
	// - 1 cancellation
	// - 2 duplicate reminders
	// - 1 fee notice
	// - 1 "kab due hai?" question
	// - Memes & jokes
	// - 1 deadline whose correction arrives 2 days later

	dateOS1 := now.AddDate(0, 0, -10)
	dateOS1Due := now.AddDate(0, 0, -7)

	dateMathsQuiz := now.AddDate(0, 0, -5)
	dateMathsQuizDue := now.AddDate(0, 0, -3)

	dateFee := now.AddDate(0, 0, -4)
	dateFeeDue := now.AddDate(0, 0, 5) // In 5 days

	dateDBMS1 := now.AddDate(0, 0, -2)
	dateDBMS1Due := now.AddDate(0, 0, 1) // Tomorrow

	dateCNLab := now.AddDate(0, 0, -1)
	dateCNLabDue := now.AddDate(0, 0, 2) // Day after tomorrow

	dateOS2 := now
	dateOS2Due := now.AddDate(0, 0, 6) // In 6 days

	lines := []chatLine{
		// Day 1: System info & banter
		{d14Ago, "System", "Messages and calls are end-to-end encrypted."},
		{d14Ago.Add(1 * time.Hour), "Rohan", "Bhai timetable mila kya?"},
		{d14Ago.Add(2 * time.Hour), "Aman", "haan group description check karo"},
		{d14Ago.Add(3 * time.Hour), "Priya", "hahaha ok lol 😂"},

		// Day 4: OS Assignment 1 released
		{dateOS1, "Prof Sharma", fmt.Sprintf("Dear students, Operating Systems Assignment 1 is uploaded. Due date is %s by 5 PM sharp.", dateOS1Due.Format("02/01/2006"))},
		{dateOS1.Add(10 * time.Minute), "Rohan", "ok sir noted"},
		{dateOS1.Add(15 * time.Minute), "Simran", "<Media omitted>"},

		// Day 6: Trap - "Assignment kab due hai?" question
		{dateOS1.Add(48 * time.Hour), "Vikas", "bhai OS assignment kab due hai? koi bata do please"},
		{dateOS1.Add(49 * time.Hour), "Pooja", "parso 5 pm tak bola tha sir ne"},

		// Day 7: Duplicate reminder for OS Assignment 1
		{dateOS1Due.Add(-24 * time.Hour), "CR Amit", fmt.Sprintf("Reminder: OS Assignment 1 last date is tomorrow (%s) 5 PM. Don't miss.", dateOS1Due.Format("02 Jan"))},

		// Day 9: Maths Quiz announcement
		{dateMathsQuiz, "Prof Verma", fmt.Sprintf("Mathematics surprise quiz scheduled for %s at 10 AM in LT-2.", dateMathsQuizDue.Format("02/01/2006"))},
		{dateMathsQuiz.Add(30 * time.Minute), "Rohan", "Sir please syllabus bata do"},
		{dateMathsQuiz.Add(35 * time.Minute), "Prof Verma", "Unit 1 and Unit 2 only."},

		// Day 10: Fee payment notice
		{dateFee, "Accounts Dept", fmt.Sprintf("Notice: Semester Exam Fee portal is active. Last date to pay fee is %s without late fine.", dateFeeDue.Format("02 Jan"))},
		{dateFee.Add(2 * time.Hour), "Aman", "Challan ya online?"},
		{dateFee.Add(3 * time.Hour), "CR Amit", "Both work fine."},

		// Day 11: Maths Quiz Postponed by 2 days (Postponement trap 1)
		{dateMathsQuiz.Add(48 * time.Hour), "Prof Verma", "Due to sports day, Mathematics quiz is postponed by 2 days. New date is Friday 10 AM."},

		// Day 12: DBMS Assignment 1 with relative date ("kal submit karna hai")
		{dateDBMS1, "Prof Rao", "DBMS Lab assignment 1: write SQL queries for schema. Submit kal subah 10 baje tak on portal."},
		{dateDBMS1.Add(5 * time.Minute), "Pooja", "ok sir"},

		// Day 13: CN Lab Viva announcement
		{dateCNLab, "Prof Iyer", "Computer Networks lab viva parso 2 PM conduct hoga."},

		// Day 13 evening: Correction arriving 2 days later - CN Lab viva shifted to 4 PM (Postponement trap 2)
		{dateCNLab.Add(6 * time.Hour), "Prof Iyer", "Correction: CN lab viva timing shifted to 4 PM (parso)."},

		// Day 14: Fee reminder (Duplicate reminder trap 2)
		{now.Add(-12 * time.Hour), "Accounts Dept", fmt.Sprintf("Again reminding: fee payment last date %s.", dateFeeDue.Format("02 Jan"))},

		// Day 14: OS Assignment 2 released
		{dateOS2, "Prof Sharma", fmt.Sprintf("Operating Systems Assignment 2 is released. Due on %s 5 PM.", dateOS2Due.Format("02 Jan"))},

		// Day 14: CN Lab Cancellation (Cancellation trap 1)
		{now.Add(-2 * time.Hour), "Prof Iyer", "Notice: Due to faculty meeting, CN lab viva is cancelled for this week."},
		{now.Add(-1 * time.Hour), "Rohan", "masti maze! 🎉"},
	}

	// 1. Build Android export
	var androidContent string
	for _, l := range lines {
		androidContent += andFormat(l.t, l.sender, l.text) + "\n"
	}
	if err := os.WriteFile(filepath.Join(outDir, "college_chat_android.txt"), []byte(androidContent), 0644); err != nil {
		return err
	}

	// 2. Build iOS export
	var iosContent string
	for _, l := range lines {
		iosContent += iosFormat(l.t, l.sender, l.text) + "\n"
	}
	if err := os.WriteFile(filepath.Join(outDir, "college_chat_ios.txt"), []byte(iosContent), 0644); err != nil {
		return err
	}

	// 3. Ground Truth JSON
	groundTruth := []GroundTruthItem{
		{
			ID:              "os-assign-1",
			Kind:            model.KindAssignment,
			Title:           "OS Assignment 1",
			Subject:         "Operating Systems",
			ExpectedDate:    dateOS1Due.Format("2006-01-02"),
			ExpectedTime:    "17:00",
			ExpectedStatus:  model.StatusActive,
			EvidenceSnippet: "Operating Systems Assignment 1 is uploaded",
			IsTrap:          false,
		},
		{
			ID:              "maths-quiz",
			Kind:            model.KindExam,
			Title:           "Mathematics quiz",
			Subject:         "Mathematics",
			ExpectedDate:    dateMathsQuizDue.AddDate(0, 0, 2).Format("2006-01-02"), // postponed
			ExpectedTime:    "10:00",
			ExpectedStatus:  model.StatusPostponed,
			EvidenceSnippet: "Mathematics surprise quiz scheduled",
			IsTrap:          false,
		},
		{
			ID:              "fee-portal",
			Kind:            model.KindFee,
			Title:           "Semester Exam Fee",
			Subject:         "Accounts",
			ExpectedDate:    dateFeeDue.Format("2006-01-02"),
			ExpectedTime:    "",
			ExpectedStatus:  model.StatusActive,
			EvidenceSnippet: "Semester Exam Fee portal is active",
			IsTrap:          false,
		},
		{
			ID:              "dbms-assign-1",
			Kind:            model.KindAssignment,
			Title:           "DBMS Lab assignment 1",
			Subject:         "DBMS",
			ExpectedDate:    dateDBMS1Due.Format("2006-01-02"),
			ExpectedTime:    "10:00",
			ExpectedStatus:  model.StatusActive,
			EvidenceSnippet: "DBMS Lab assignment 1",
			IsTrap:          false,
		},
		{
			ID:              "os-assign-2",
			Kind:            model.KindAssignment,
			Title:           "OS Assignment 2",
			Subject:         "Operating Systems",
			ExpectedDate:    dateOS2Due.Format("2006-01-02"),
			ExpectedTime:    "17:00",
			ExpectedStatus:  model.StatusActive,
			EvidenceSnippet: "Operating Systems Assignment 2 is released",
			IsTrap:          false,
		},
		{
			ID:              "cn-lab-viva",
			Kind:            model.KindCancellation,
			Title:           "CN lab viva",
			Subject:         "Computer Networks",
			ExpectedDate:    dateCNLabDue.Format("2006-01-02"),
			ExpectedTime:    "16:00",
			ExpectedStatus:  model.StatusCancelled,
			EvidenceSnippet: "CN lab viva is cancelled",
			IsTrap:          true,
		},
	}

	gtBytes, err := json.MarshalIndent(groundTruth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "ground_truth.json"), gtBytes, 0644)
}
