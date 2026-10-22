package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// DateFormatOrder represents whether DD/MM or MM/DD is preferred
type DateFormatOrder int

const (
	DateFormatAuto DateFormatOrder = iota
	DateFormatDMY
	DateFormatMDY
)

// ParseOptions configures chat parsing
type ParseOptions struct {
	GroupID     string
	Location    *time.Location
	DateFormat  DateFormatOrder
	WarnHandler func(string)
}

// Result contains parsed messages and parsing metadata
type Result struct {
	Messages         []model.Message
	InferredFormat   string
	AmbiguousWarning string
}

// Regex patterns for Android and iOS formats
// Android: "12/10/25, 9:41 pm - Name: text" or "12/10/2025, 21:41 - Name: text"
var androidHeaderRegex = regexp.MustCompile(`^(\d{1,2}[/.-]\d{1,2}[/.-]\d{2,4}),?\s+(\d{1,2}:\d{2}(?::\d{2})?(?:\s*[aApP][mM])?)\s*-\s*(.*)$`)

// iOS: "[12/10/25, 9:41:03 PM] Name: text" or "[12/10/2025, 21:41:03] Name: text"
// Also supports non-breaking space / thin space (\u202f, \u00a0)
var iosHeaderRegex = regexp.MustCompile(`^\[(\d{1,2}[/.-]\d{1,2}[/.-]\d{2,4}),?\s+([\d:]{5,8}(?:[\s\x{202f}\x{00a0}]*[aApP][mM])?)\]\s*(.*)$`)

// System / noise patterns
var (
	mediaOmittedRegex = regexp.MustCompile(`(?i)<media omitted>|image omitted|video omitted|audio omitted|sticker omitted|document omitted`)
	deletedRegex      = regexp.MustCompile(`(?i)this message was deleted|you deleted this message`)
	systemActionRegex = regexp.MustCompile(`(?i)(Messages and calls are end-to-end encrypted|created group|added|left|removed|changed the subject|changed this group's icon|security code changed)`)
)

// ParseChat parses an exported WhatsApp chat text stream
func ParseChat(r io.Reader, opts ParseOptions) (*Result, error) {
	if opts.Location == nil {
		opts.Location = time.FixedZone("IST", 5*3600+30*60) // Default IST +05:30
	}
	if opts.GroupID == "" {
		opts.GroupID = "default"
	}

	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	// 1. Scan whole file to infer DateFormat (DD/MM vs MM/DD)
	order, ambiguous := inferDateFormat(lines, opts.DateFormat)
	var warnMsg string
	if ambiguous {
		warnMsg = "Date order DD/MM vs MM/DD is ambiguous in export; defaulted to DD/MM/YY (standard for Indian college groups)"
		if opts.WarnHandler != nil {
			opts.WarnHandler(warnMsg)
		}
	}

	// 2. Collate lines into multi-line message records
	type rawEntry struct {
		dateStr   string
		timeStr   string
		remainder string
		extra     []string
	}

	var entries []rawEntry

	for _, rawLine := range lines {
		// Clean invisible BOM and control chars
		line := strings.TrimPrefix(rawLine, "\ufeff")
		line = strings.TrimPrefix(line, "\u200e") // left-to-right mark

		if match := iosHeaderRegex.FindStringSubmatch(line); match != nil {
			entries = append(entries, rawEntry{
				dateStr:   match[1],
				timeStr:   match[2],
				remainder: match[3],
			})
			continue
		}

		if match := androidHeaderRegex.FindStringSubmatch(line); match != nil {
			entries = append(entries, rawEntry{
				dateStr:   match[1],
				timeStr:   match[2],
				remainder: match[3],
			})
			continue
		}

		if len(entries) > 0 {
			// Continuation line for previous message
			entries[len(entries)-1].extra = append(entries[len(entries)-1].extra, line)
		}
	}

	// 3. Process each entry into a model.Message
	messages := make([]model.Message, 0, len(entries))
	for _, entry := range entries {
		t, err := parseDateTime(entry.dateStr, entry.timeStr, order, opts.Location)
		if err != nil {
			continue // Skip unparseable lines
		}

		// Split remainder into sender and text
		sender, text, isSystem := splitSenderAndText(entry.remainder)
		if len(entry.extra) > 0 {
			if text != "" {
				text = text + "\n" + strings.Join(entry.extra, "\n")
			} else {
				text = strings.Join(entry.extra, "\n")
			}
		}

		isMedia := mediaOmittedRegex.MatchString(text)
		isDeleted := deletedRegex.MatchString(text)
		if isSystem || systemActionRegex.MatchString(text) {
			isSystem = true
		}

		// Normalized text for consistent hashing
		normText := strings.TrimSpace(text)
		msgID := ComputeMessageID(t, sender, normText)

		messages = append(messages, model.Message{
			ID:        msgID,
			GroupID:   opts.GroupID,
			Timestamp: t,
			Sender:    sender,
			Text:      normText,
			RawText:   text,
			IsSystem:  isSystem,
			IsMedia:   isMedia,
			IsDeleted: isDeleted,
		})
	}

	formatStr := "DD/MM/YY"
	if order == DateFormatMDY {
		formatStr = "MM/DD/YY"
	}

	return &Result{
		Messages:         messages,
		InferredFormat:   formatStr,
		AmbiguousWarning: warnMsg,
	}, nil
}

// ComputeMessageID calculates a stable SHA-256 hash
func ComputeMessageID(t time.Time, sender, text string) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d|%s|%s", t.Unix(), strings.TrimSpace(sender), strings.TrimSpace(text))))
	return hex.EncodeToString(h.Sum(nil))
}

func splitSenderAndText(remainder string) (string, string, bool) {
	colonIdx := strings.Index(remainder, ": ")
	if colonIdx == -1 {
		// No colon => system notification like "Alice joined", "Messages are end-to-end encrypted"
		return "System", remainder, true
	}
	sender := strings.TrimSpace(remainder[:colonIdx])
	text := strings.TrimSpace(remainder[colonIdx+2:])
	return sender, text, false
}

// inferDateFormat inspects date tokens across the chat to check if first or second number exceeds 12
func inferDateFormat(lines []string, requested DateFormatOrder) (DateFormatOrder, bool) {
	if requested == DateFormatDMY {
		return DateFormatDMY, false
	}
	if requested == DateFormatMDY {
		return DateFormatMDY, false
	}

	firstExceeds12 := false
	secondExceeds12 := false

	dateRegex := regexp.MustCompile(`(?:^|\[)(\d{1,2})[/.-](\d{1,2})[/.-](\d{2,4})`)
	for _, l := range lines {
		m := dateRegex.FindStringSubmatch(l)
		if m != nil {
			v1, _ := strconv.Atoi(m[1])
			v2, _ := strconv.Atoi(m[2])
			if v1 > 12 {
				firstExceeds12 = true
			}
			if v2 > 12 {
				secondExceeds12 = true
			}
		}
	}

	if firstExceeds12 && !secondExceeds12 {
		return DateFormatDMY, false // First component is Day (>12)
	}
	if secondExceeds12 && !firstExceeds12 {
		return DateFormatMDY, false // Second component is Day (>12)
	}

	// Ambiguous: default to DD/MM/YY (standard for Indian college chats) with warning
	return DateFormatDMY, true
}

func parseDateTime(dateStr, timeStr string, order DateFormatOrder, loc *time.Location) (time.Time, error) {
	// Normalize separators to slash
	dateStr = strings.ReplaceAll(dateStr, "-", "/")
	dateStr = strings.ReplaceAll(dateStr, ".", "/")

	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid date parts: %s", dateStr)
	}

	var d, m, y int
	p1, _ := strconv.Atoi(parts[0])
	p2, _ := strconv.Atoi(parts[1])
	p3, _ := strconv.Atoi(parts[2])

	if order == DateFormatMDY {
		m, d = p1, p2
	} else {
		d, m = p1, p2
	}

	if p3 < 100 {
		y = 2000 + p3
	} else {
		y = p3
	}

	// Normalize timeStr (handling thin spaces \u202f, \u00a0, lowercase/uppercase am/pm)
	cleanTime := strings.ReplaceAll(timeStr, "\u202f", " ")
	cleanTime = strings.ReplaceAll(cleanTime, "\u00a0", " ")
	cleanTime = strings.TrimSpace(cleanTime)

	timeParts := strings.Fields(cleanTime)
	var hour, min, sec int
	var isPM bool
	var hasAmPm bool

	clock := cleanTime
	if len(timeParts) == 2 {
		clock = timeParts[0]
		ampm := strings.ToUpper(timeParts[1])
		if ampm == "PM" {
			isPM = true
			hasAmPm = true
		} else if ampm == "AM" {
			hasAmPm = true
		}
	} else if strings.HasSuffix(strings.ToUpper(cleanTime), "PM") {
		clock = strings.TrimSpace(cleanTime[:len(cleanTime)-2])
		isPM = true
		hasAmPm = true
	} else if strings.HasSuffix(strings.ToUpper(cleanTime), "AM") {
		clock = strings.TrimSpace(cleanTime[:len(cleanTime)-2])
		hasAmPm = true
	}

	sub := strings.Split(clock, ":")
	if len(sub) >= 2 {
		hour, _ = strconv.Atoi(sub[0])
		min, _ = strconv.Atoi(sub[1])
		if len(sub) >= 3 {
			sec, _ = strconv.Atoi(sub[2])
		}
	}

	if hasAmPm {
		if isPM && hour < 12 {
			hour += 12
		} else if !isPM && hour == 12 {
			hour = 0
		}
	}

	return time.Date(y, time.Month(m), d, hour, min, sec, 0, loc), nil
}
