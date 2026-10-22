package prefilter

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/aalsi-app/aalsi/pkg/model"
)

// Regex patterns to detect dates, times, days, numbers and Hinglish/Hindi keywords
var (
	// Number or date token: e.g. "18th", "28", "2025", "10/12", "10-12", "3 pm", "15:00"
	dateOrNumberRegex = regexp.MustCompile(`(?i)\b\d{1,4}(?:st|nd|rd|th)?\b|\b\d{1,2}[:.]\d{2}\b|\b\d{1,2}[/-]\d{1,2}\b`)

	// Weekdays in English & Hindi/Hinglish
	weekdayRegex = regexp.MustCompile(`(?i)\b(monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|wed|thu|fri|sat|sun|somwar|mangalwar|budhwar|guruwar|shukrawar|shaniwar|ravivar|som|mangal|budh|guru|shukra|shani|ravi)\b`)

	// Hindi & Hinglish temporal relative words
	temporalHindiRegex = regexp.MustCompile(`(?i)\b(aaj|kal|parso|tarso|aaj-kal|agle|agla|agli|pichle|pichla|hafte|mahine|subah|shaam|dopahar|raat|baje|tak|eod|today|tomorrow|yesterday|tonight|next\s+week|this\s+week)\b`)

	// Academic & action deadline keywords
	actionKeywordsRegex = regexp.MustCompile(`(?i)\b(due|deadline|submit|submission|assignment|project|exam|quiz|midsem|endsem|test|viva|lab|presentation|fee|fees|fine|challan|form|registration|shifted|postponed|preponed|cancelled|canceled|extended|rescheduled|timing|venue|hall|admit\s*card|hall\s*ticket|last\s*date|reminder|reminding|dhyan|jaldi|karo|bhejo|jama)\b`)

	// Devanagari script regex: range \x{0900}-\x{097F}
	devanagariRegex = regexp.MustCompile(`[\x{0900}-\x{097F}]`)

	// Noise: single-word acknowledgements and chat clutter
	singleNoiseWords = map[string]bool{
		"ok": true, "k": true, "kk": true, "okk": true, "ok sir": true, "okay": true, "noted": true,
		"yes": true, "no": true, "haan": true, "nahi": true, "theek": true, "theek hai": true, "thik h": true,
		"lol": true, "lmao": true, "haha": true, "hahaha": true, "rofl": true,
		"thanks": true, "thank you": true, "ty": true, "dhanyawad": true, "shukriya": true,
		"gud mrng": true, "good morning": true, "good night": true, "gm": true, "gn": true,
		"+1": true, "done": true, "yes sir": true, "no sir": true, "ha sir": true,
	}
)

// ShouldKeep evaluates whether a message should be retained for LLM extraction
// Design rule: Drop obvious noise, but NEVER drop a message if it contains a date/time/number or action token.
func ShouldKeep(m model.Message) bool {
	if m.IsSystem || m.IsMedia || m.IsDeleted {
		return false
	}

	text := strings.TrimSpace(m.Text)
	if text == "" {
		return false
	}

	// Always retain if message contains any date/time pattern or number token
	hasDateOrNumber := dateOrNumberRegex.MatchString(text)
	hasWeekday := weekdayRegex.MatchString(text)
	hasTemporal := temporalHindiRegex.MatchString(text)
	hasAction := actionKeywordsRegex.MatchString(text)

	// If it has any temporal, action or date cue, we definitely keep it
	if hasDateOrNumber || hasWeekday || hasTemporal || hasAction {
		return true
	}

	// Devanagari script detection: if written in Hindi script, keep for semantic check
	if devanagariRegex.MatchString(text) {
		return true
	}

	// Check if entire text is a short acknowledgment noise
	lowerClean := strings.ToLower(text)
	lowerClean = strings.Trim(lowerClean, ".!?,~ ")
	if singleNoiseWords[lowerClean] {
		return false
	}

	// Drop messages that are strictly non-alphanumeric laughter/emojis
	if isOnlyEmojisOrPunctuation(text) {
		return false
	}

	// If message is very short (< 15 chars) and matched no action/time pattern, drop as chat chatter
	if len(text) < 15 {
		return false
	}

	// Long messages might be general discussion or notices
	return true
}

func isOnlyEmojisOrPunctuation(s string) bool {
	hasAlphanum := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanum = true
			break
		}
	}
	return !hasAlphanum
}

// FilterMessages filters a slice of messages, returning only candidate messages
func FilterMessages(messages []model.Message) []model.Message {
	filtered := make([]model.Message, 0, len(messages))
	for _, m := range messages {
		if ShouldKeep(m) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
