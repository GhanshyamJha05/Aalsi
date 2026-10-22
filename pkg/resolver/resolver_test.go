package resolver

import (
	"testing"
	"time"
)

func TestResolveDate_HinglishAndRelatives(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	// Wednesday, Oct 15, 2025 at 14:00 IST
	ref := time.Date(2025, 10, 15, 14, 0, 0, 0, loc)

	tests := []struct {
		name            string
		rawDate         string
		rawTime         string
		context         string
		expectedYear    int
		expectedMonth   time.Month
		expectedDay     int
		expectedHour    int
		expectedMin     int
		expectedConfMin float64
		needsReview     bool
	}{
		{
			name:            "aaj (today) with explicit time",
			rawDate:         "aaj",
			rawTime:         "5 PM",
			context:         "aaj 5 pm tak assignment submit karna hai",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     15,
			expectedHour:    17,
			expectedMin:     0,
			expectedConfMin: 0.8,
			needsReview:     false,
		},
		{
			name:            "kal (tomorrow) with future cues",
			rawDate:         "kal",
			rawTime:         "11:59 pm",
			context:         "kal submit karna hai sabko",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     16,
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.8,
			needsReview:     false,
		},
		{
			name:            "kal with past tense cues (yesterday)",
			rawDate:         "kal",
			rawTime:         "",
			context:         "kal submit kar diya tha maine",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     14,
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.6,
			needsReview:     false,
		},
		{
			name:            "kal ambiguous (flagged for review)",
			rawDate:         "kal",
			rawTime:         "",
			context:         "kal",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     16, // defaults tomorrow
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.4,
			needsReview:     true,
		},
		{
			name:            "parso (day after tomorrow)",
			rawDate:         "parso",
			rawTime:         "3 PM",
			context:         "lab shifted to parso 3 pm",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     17,
			expectedHour:    15,
			expectedMin:     0,
			expectedConfMin: 0.8,
			needsReview:     false,
		},
		{
			name:            "Specific day month word: 28th Oct",
			rawDate:         "28th Oct",
			rawTime:         "",
			context:         "fee payment last date 28th Oct",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     28,
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.85,
			needsReview:     false,
		},
		{
			name:            "Day only: 20th",
			rawDate:         "20th",
			rawTime:         "5 pm",
			context:         "quiz on 20th 5 pm",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     20,
			expectedHour:    17,
			expectedMin:     0,
			expectedConfMin: 0.85,
			needsReview:     false,
		},
		{
			name:            "next Monday tak",
			rawDate:         "next Monday tak",
			rawTime:         "",
			context:         "next Monday tak submission portal open rahega",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     20, // Oct 15 is Wed; next Mon is Oct 20
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.8,
			needsReview:     false,
		},
		{
			name:            "Hinglish weekday: shukrawar",
			rawDate:         "shukrawar",
			rawTime:         "",
			context:         "shukrawar tak assignment dena hai",
			expectedYear:    2025,
			expectedMonth:   time.October,
			expectedDay:     17, // Friday Oct 17
			expectedHour:    23,
			expectedMin:     59,
			expectedConfMin: 0.8,
			needsReview:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ResolveDate(tt.rawDate, tt.rawTime, tt.context, ref)
			if res.ResolvedDate.Year() != tt.expectedYear ||
				res.ResolvedDate.Month() != tt.expectedMonth ||
				res.ResolvedDate.Day() != tt.expectedDay {
				t.Errorf("Date mismatch: got %04d-%02d-%02d, want %04d-%02d-%02d (%s)",
					res.ResolvedDate.Year(), res.ResolvedDate.Month(), res.ResolvedDate.Day(),
					tt.expectedYear, tt.expectedMonth, tt.expectedDay, res.Explanation)
			}
			if res.ResolvedDate.Hour() != tt.expectedHour || res.ResolvedDate.Minute() != tt.expectedMin {
				t.Errorf("Time mismatch: got %02d:%02d, want %02d:%02d",
					res.ResolvedDate.Hour(), res.ResolvedDate.Minute(),
					tt.expectedHour, tt.expectedMin)
			}
			if res.Confidence < tt.expectedConfMin {
				t.Errorf("Confidence too low: got %v, expected >= %v", res.Confidence, tt.expectedConfMin)
			}
			if res.NeedsReview != tt.needsReview {
				t.Errorf("NeedsReview mismatch: got %v, want %v", res.NeedsReview, tt.needsReview)
			}
		})
	}
}
