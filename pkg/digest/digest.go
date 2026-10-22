package digest

import (
	"fmt"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// Language represents the selected digest language
type Language string

const (
	LangEnglish  Language = "en"
	LangHindi    Language = "hi"
	LangHinglish Language = "hinglish"
)

// Digest holds organized deadlines for morning display
type Digest struct {
	Language       Language             `json:"language"`
	GeneratedAt    time.Time            `json:"generated_at"`
	Summary        string               `json:"summary"` // 2 lines
	ExtractionMode string               `json:"extraction_mode"` // "Gemma 3 12B" or "rule mode"
	Today          []model.DeadlineItem `json:"today"`
	Tomorrow       []model.DeadlineItem `json:"tomorrow"`
	ThisWeek       []model.DeadlineItem `json:"this_week"`
	YouMayHaveMissed []model.DeadlineItem `json:"you_may_have_missed"`
	ReviewQueue    []model.DeadlineItem `json:"review_queue"`
}

// Generate creates a structured morning digest from active items relative to now
func Generate(items []model.DeadlineItem, now time.Time, lang Language) *Digest {
	if lang == "" {
		lang = LangEnglish
	}
	loc := now.Location()

	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfToday := startOfToday.AddDate(0, 0, 1)
	endOfTomorrow := endOfToday.AddDate(0, 0, 1)
	endOfWeek := startOfToday.AddDate(0, 0, 7)

	hasGemma := false
	hasRule := false

	d := &Digest{
		Language:    lang,
		GeneratedAt: now,
	}

	for _, it := range items {
		if it.ExtractionMode == "gemma" {
			hasGemma = true
		} else {
			hasRule = true
		}

		if it.Status == model.StatusCancelled || it.Status == model.StatusDismissed {
			continue
		}

		if it.NeedsReview {
			d.ReviewQueue = append(d.ReviewQueue, it)
			continue
		}

		due := it.DueDate
		if due.Before(startOfToday) {
			d.YouMayHaveMissed = append(d.YouMayHaveMissed, it)
		} else if due.Before(endOfToday) {
			d.Today = append(d.Today, it)
		} else if due.Before(endOfTomorrow) {
			d.Tomorrow = append(d.Tomorrow, it)
		} else if due.Before(endOfWeek) {
			d.ThisWeek = append(d.ThisWeek, it)
		}
	}

	if hasGemma {
		d.ExtractionMode = "Gemma 3 12B"
	} else if hasRule {
		d.ExtractionMode = "rule mode"
	} else {
		d.ExtractionMode = "rule mode"
	}

	d.Summary = buildSummary(d, lang)
	return d
}

func buildSummary(d *Digest, lang Language) string {
	todayCount := len(d.Today)
	tomorrowCount := len(d.Tomorrow)
	missedCount := len(d.YouMayHaveMissed)
	weekCount := len(d.ThisWeek)

	switch lang {
	case LangHindi:
		var line1 string
		if todayCount == 0 && tomorrowCount == 0 {
			line1 = "आज और कल के लिए कोई काम निर्धारित नहीं है।"
		} else if todayCount == 1 && tomorrowCount == 1 {
			line1 = "आज आपके लिए 1 काम निर्धारित है, और कल 1 काम है।"
		} else if todayCount == 1 {
			line1 = fmt.Sprintf("आज आपके लिए 1 काम निर्धारित है, और कल %d काम हैं।", tomorrowCount)
		} else if tomorrowCount == 1 {
			line1 = fmt.Sprintf("आज आपके लिए %d काम निर्धारित हैं, और कल 1 काम है।", todayCount)
		} else {
			line1 = fmt.Sprintf("आज आपके लिए %d काम निर्धारित हैं, और कल %d काम हैं।", todayCount, tomorrowCount)
		}

		var line2 string
		if missedCount == 1 {
			line2 = fmt.Sprintf("ध्यान दें: 1 पुराना काम शायद छूट गया है। इस हफ्ते कुल %d काम हैं।", weekCount)
		} else if missedCount > 1 {
			line2 = fmt.Sprintf("ध्यान दें: %d पुराने काम शायद छूट गए हैं। इस हफ्ते कुल %d काम हैं।", missedCount, weekCount)
		} else if weekCount == 1 {
			line2 = "शानदार! कोई पुराना काम बकाया नहीं है। इस हफ्ते केवल 1 अन्य काम है।"
		} else {
			line2 = fmt.Sprintf("शानदार! कोई पुराना काम बकाया नहीं है। इस हफ्ते कुल %d काम हैं।", weekCount)
		}
		return line1 + "\n" + line2

	case LangHinglish:
		var line1 string
		if todayCount == 0 && tomorrowCount == 0 {
			line1 = "Aaj aur kal koi deadlines nahi hain."
		} else if todayCount == 1 && tomorrowCount == 1 {
			line1 = "Aaj ke liye 1 deadline hai, aur kal 1 item due hai."
		} else if todayCount == 1 {
			line1 = fmt.Sprintf("Aaj ke liye 1 deadline hai, aur kal %d items due hain.", tomorrowCount)
		} else if tomorrowCount == 1 {
			line1 = fmt.Sprintf("Aaj ke liye %d deadlines hain, aur kal 1 item due hai.", todayCount)
		} else {
			line1 = fmt.Sprintf("Aaj ke liye %d deadlines hain, aur kal %d items due hain.", todayCount, tomorrowCount)
		}

		var line2 string
		if missedCount == 1 {
			line2 = fmt.Sprintf("Alert: 1 missed task check karlo. Is hafte total %d deadlines hain.", weekCount)
		} else if missedCount > 1 {
			line2 = fmt.Sprintf("Alert: %d missed tasks check karlo. Is hafte total %d deadlines hain.", missedCount, weekCount)
		} else if weekCount == 1 {
			line2 = "Sab chill hai, koi missed task nahi hai. Is week total 1 deadline hai."
		} else {
			line2 = fmt.Sprintf("Sab chill hai, koi missed task nahi hai. Is week total %d deadlines hain.", weekCount)
		}
		return line1 + "\n" + line2

	default: // English
		var line1 string
		if todayCount == 0 && tomorrowCount == 0 {
			line1 = "You have no deadlines due today and tomorrow is clear."
		} else if todayCount == 1 && tomorrowCount == 1 {
			line1 = "You have 1 deadline due today and 1 scheduled for tomorrow."
		} else if todayCount == 1 {
			line1 = fmt.Sprintf("You have 1 deadline due today and %s scheduled for tomorrow.", pluralize(tomorrowCount, "deadline"))
		} else if tomorrowCount == 1 {
			line1 = fmt.Sprintf("You have %s due today and 1 scheduled for tomorrow.", pluralize(todayCount, "deadline"))
		} else {
			line1 = fmt.Sprintf("You have %s due today and %s scheduled for tomorrow.", pluralize(todayCount, "deadline"), pluralize(tomorrowCount, "deadline"))
		}

		var line2 string
		if missedCount == 1 {
			line2 = fmt.Sprintf("Heads up: 1 overdue item may have been missed. You have %s this week.", pluralize(weekCount, "other task"))
		} else if missedCount > 1 {
			line2 = fmt.Sprintf("Heads up: %d overdue items may have been missed. You have %s this week.", missedCount, pluralize(weekCount, "other task"))
		} else if weekCount == 1 {
			line2 = "All caught up on past tasks. 1 other upcoming deadline this week."
		} else {
			line2 = fmt.Sprintf("All caught up on past tasks. %s upcoming this week.", pluralize(weekCount, "other deadline"))
		}
		return line1 + "\n" + line2
	}
}

func pluralize(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	// Simple English pluralizer
	if strings.HasSuffix(singular, "s") || strings.HasSuffix(singular, "sh") || strings.HasSuffix(singular, "ch") {
		return fmt.Sprintf("%d %ses", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

// FormatPlainText creates a clean clipboard-ready text format for self-messaging
func (d *Digest) FormatPlainText() string {
	var sb strings.Builder
	sb.WriteString("📋 *Aalsi Morning Digest*\n")
	sb.WriteString(fmt.Sprintf("_%s_\n\n", d.GeneratedAt.Format("Monday, 02 Jan 2006")))
	sb.WriteString(d.Summary)
	sb.WriteString("\n\n")

	if len(d.Today) > 0 {
		sb.WriteString("🔴 *TODAY*\n")
		for _, it := range d.Today {
			timeStr := it.DueDate.Format("03:04 PM")
			if !it.HasExplicitTime {
				timeStr = "EOD"
			}
			sub := ""
			if it.SubjectOrCourse != "" {
				sub = fmt.Sprintf("[%s] ", it.SubjectOrCourse)
			}
			sb.WriteString(fmt.Sprintf("• %s%s (by %s)\n", sub, it.Title, timeStr))
		}
		sb.WriteString("\n")
	}

	if len(d.Tomorrow) > 0 {
		sb.WriteString("🟡 *TOMORROW*\n")
		for _, it := range d.Tomorrow {
			sub := ""
			if it.SubjectOrCourse != "" {
				sub = fmt.Sprintf("[%s] ", it.SubjectOrCourse)
			}
			sb.WriteString(fmt.Sprintf("• %s%s\n", sub, it.Title))
		}
		sb.WriteString("\n")
	}

	if len(d.ThisWeek) > 0 {
		sb.WriteString("🔵 *THIS WEEK*\n")
		for _, it := range d.ThisWeek {
			sub := ""
			if it.SubjectOrCourse != "" {
				sub = fmt.Sprintf("[%s] ", it.SubjectOrCourse)
			}
			sb.WriteString(fmt.Sprintf("• %s%s — %s\n", sub, it.Title, it.DueDate.Format("Mon, 02 Jan")))
		}
		sb.WriteString("\n")
	}

	if len(d.YouMayHaveMissed) > 0 {
		sb.WriteString("⚠️ *YOU MAY HAVE MISSED*\n")
		for _, it := range d.YouMayHaveMissed {
			sub := ""
			if it.SubjectOrCourse != "" {
				sub = fmt.Sprintf("[%s] ", it.SubjectOrCourse)
			}
			sb.WriteString(fmt.Sprintf("• %s%s (was due %s)\n", sub, it.Title, it.DueDate.Format("02 Jan")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("_Aalsi did the reading. You do the doing._")
	return sb.String()
}
