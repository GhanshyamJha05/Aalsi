package ics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// GenerateCalendar creates an RFC 5545 compliant iCalendar string
// Only confirmed or high-confidence non-cancelled items are included
func GenerateCalendar(items []model.DeadlineItem) string {
	var sb strings.Builder

	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//Aalsi//Aalsi Deadline Catcher 1.0//EN\r\n")
	sb.WriteString("CALSCALE:GREGORIAN\r\n")
	sb.WriteString("METHOD:PUBLISH\r\n")
	sb.WriteString("X-WR-CALNAME:Aalsi College Deadlines\r\n")
	sb.WriteString("X-WR-TIMEZONE:Asia/Kolkata\r\n")

	nowUTC := time.Now().UTC().Format("20060102T150405Z")

	for _, it := range items {
		// Include only high-confidence or confirmed items, excluding cancelled / review items
		if it.Status == model.StatusCancelled || it.Status == model.StatusDismissed || it.NeedsReview {
			continue
		}

		uid := fmt.Sprintf("%s@aalsi.local", it.ID)
		summary := it.Title
		if it.SubjectOrCourse != "" {
			summary = fmt.Sprintf("[%s] %s", it.SubjectOrCourse, it.Title)
		}

		// Prepare description with original message evidence
		var descBuilder strings.Builder
		if it.ActionNeeded != "" {
			descBuilder.WriteString(fmt.Sprintf("Action: %s\\n\\n", escapeICS(it.ActionNeeded)))
		}
		descBuilder.WriteString("--- Evidence from WhatsApp ---\\n")
		for _, ev := range it.Evidence {
			descBuilder.WriteString(fmt.Sprintf("[%s] %s: %s\\n",
				ev.Timestamp.Format("02 Jan 15:04"),
				escapeICS(ev.Sender),
				escapeICS(ev.MessageText),
			))
		}
		descBuilder.WriteString("\\nAalsi did the reading. You do the doing.")

		dueUTC := it.DueDate.UTC().Format("20060102T150405Z")
		// End time: 1 hour after or same day
		endUTC := it.DueDate.Add(1 * time.Hour).UTC().Format("20060102T150405Z")

		sb.WriteString("BEGIN:VEVENT\r\n")
		sb.WriteString(fmt.Sprintf("UID:%s\r\n", uid))
		sb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", nowUTC))
		sb.WriteString(fmt.Sprintf("DTSTART:%s\r\n", dueUTC))
		sb.WriteString(fmt.Sprintf("DTEND:%s\r\n", endUTC))
		sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(summary)))
		sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", descBuilder.String()))
		sb.WriteString(fmt.Sprintf("CATEGORIES:%s\r\n", strings.ToUpper(string(it.Kind))))
		sb.WriteString("STATUS:CONFIRMED\r\n")

		// 1-Day Reminder Alarm
		sb.WriteString("BEGIN:VALARM\r\n")
		sb.WriteString("TRIGGER:-P1D\r\n")
		sb.WriteString("ACTION:DISPLAY\r\n")
		sb.WriteString(fmt.Sprintf("DESCRIPTION:Reminder: %s due tomorrow!\r\n", escapeICS(summary)))
		sb.WriteString("END:VALARM\r\n")

		// 1-Hour Reminder Alarm
		sb.WriteString("BEGIN:VALARM\r\n")
		sb.WriteString("TRIGGER:-PT1H\r\n")
		sb.WriteString("ACTION:DISPLAY\r\n")
		sb.WriteString(fmt.Sprintf("DESCRIPTION:Final Call: %s due in 1 hour!\r\n", escapeICS(summary)))
		sb.WriteString("END:VALARM\r\n")

		sb.WriteString("END:VEVENT\r\n")
	}

	sb.WriteString("END:VCALENDAR\r\n")
	return sb.String()
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// ICSHash generates a fingerprint of the calendar content
func ICSHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
