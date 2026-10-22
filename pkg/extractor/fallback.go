package extractor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// FallbackRuleExtractor extracts events using deterministic rules when Ollama is offline
// This ensures students can immediately test and see extracted deadlines even before installing Ollama
func FallbackRuleExtractor(messages []model.Message) []model.RawExtractedEvent {
	var events []model.RawExtractedEvent

	// Patterns to identify subject/course
	courseRegex := regexp.MustCompile(`(?i)\b(Operating Systems|OS|Database Systems|DBMS|Mathematics|Maths|Computer Networks|CN|Data Structures|DSA|Algorithms|AI|ML)\b`)

	// Patterns for kinds
	examRegex := regexp.MustCompile(`(?i)\b(exam|quiz|midsem|endsem|test|viva)\b`)
	feeRegex := regexp.MustCompile(`(?i)\b(fee|fees|challan|payment)\b`)
	cancelRegex := regexp.MustCompile(`(?i)\b(cancelled|canceled)\b`)
	postponeRegex := regexp.MustCompile(`(?i)\b(postponed|postpone|shifted|extended|rescheduled)\b`)
	assignRegex := regexp.MustCompile(`(?i)\b(assignment|project|lab|homework|submission|report)\b`)

	// Date pattern matches
	datePattern := regexp.MustCompile(`(?i)\b(\d{1,2}(?:st|nd|rd|th)?\s+(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?))\b|\b(\d{1,2}[/-]\d{1,2}(?:[/-]\d{2,4})?)\b|\b(aaj|kal|parso|today|tomorrow|yesterday|friday|monday|tuesday|wednesday|thursday|saturday|sunday)\b`)
	timePattern := regexp.MustCompile(`(?i)\b(\d{1,2}(?::\d{2})?\s*(?:am|pm|baje))\b`)

	for _, m := range messages {
		text := m.Text
		lower := strings.ToLower(text)

		// Ignore questions like "assignment kab due hai?"
		if strings.Contains(lower, "kab due") || strings.Contains(lower, "kab hai") || strings.Contains(lower, "when is") || strings.Contains(lower, "?") {
			continue
		}

		// Detect Kind
		var kind model.EventKind
		switch {
		case cancelRegex.MatchString(lower):
			kind = model.KindCancellation
		case postponeRegex.MatchString(lower):
			kind = model.KindPostponement
		case examRegex.MatchString(lower):
			kind = model.KindExam
		case feeRegex.MatchString(lower):
			kind = model.KindFee
		case assignRegex.MatchString(lower):
			kind = model.KindAssignment
		default:
			// If message has action or date, treat as notice
			if datePattern.MatchString(lower) && strings.Contains(lower, "due") {
				kind = model.KindNotice
			} else {
				continue
			}
		}

		// Extract subject
		var subject *string
		if mCourse := courseRegex.FindString(text); mCourse != "" {
			cleanCourse := normalizeCourse(mCourse)
			subject = &cleanCourse
		}

		// Extract verbatim date expression
		rawDate := "today"
		if mDate := datePattern.FindString(text); mDate != "" {
			rawDate = mDate
		} else if kind == model.KindCancellation {
			rawDate = "today"
		} else {
			continue // No date expression found
		}

		// Extract time
		var rawTime *string
		if mTime := timePattern.FindString(text); mTime != "" {
			rawTime = &mTime
		}

		// Title generation
		title := generateFallbackTitle(kind, subject, text)

		var refersTo *string
		if kind == model.KindPostponement || kind == model.KindCancellation {
			refers := title
			refersTo = &refers
		}

		events = append(events, model.RawExtractedEvent{
			Kind:               kind,
			Title:              title,
			SubjectOrCourse:    subject,
			RawDateExpression:  rawDate,
			RawTimeExpression:  rawTime,
			RefersToEarlier:    refersTo,
			SourceMessageIDs:   []string{m.ID},
			Confidence:         0.85,
			ExtractionMode:     "rule",
		})
	}

	return events
}

func normalizeCourse(s string) string {
	lower := strings.ToLower(s)
	switch lower {
	case "os", "operating systems":
		return "Operating Systems"
	case "dbms", "database systems":
		return "DBMS"
	case "maths", "mathematics":
		return "Mathematics"
	case "cn", "computer networks":
		return "Computer Networks"
	case "dsa", "data structures":
		return "Data Structures"
	default:
		return s
	}
}

func generateFallbackTitle(kind model.EventKind, subject *string, text string) string {
	sub := ""
	if subject != nil {
		sub = *subject + " "
	}

	// Look for assignment number
	assignNumRegex := regexp.MustCompile(`(?i)(?:assignment|lab|quiz|project)\s*#?(\d+)`)
	if m := assignNumRegex.FindStringSubmatch(text); len(m) > 1 {
		return fmt.Sprintf("%s%s %s", sub, strings.Title(string(kind)), m[1])
	}

	switch kind {
	case model.KindFee:
		return "Semester Exam Fee"
	case model.KindCancellation:
		return fmt.Sprintf("%sLab Viva", sub)
	case model.KindPostponement:
		if strings.Contains(strings.ToLower(text), "quiz") {
			return fmt.Sprintf("%sQuiz", sub)
		}
		return fmt.Sprintf("%sPostponement", sub)
	case model.KindExam:
		if strings.Contains(strings.ToLower(text), "quiz") {
			return fmt.Sprintf("%sQuiz", sub)
		}
		return fmt.Sprintf("%sExam", sub)
	default:
		return fmt.Sprintf("%s%s", sub, strings.Title(string(kind)))
	}
}
