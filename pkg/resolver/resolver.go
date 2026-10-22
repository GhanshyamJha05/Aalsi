package resolver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ResolutionResult holds the resolved date and confidence
type ResolutionResult struct {
	ResolvedDate    time.Time
	HasExplicitTime bool
	Confidence      float64
	NeedsReview     bool
	Explanation     string
}

// Regex helpers
var (
	// Matches specific dates like "18th", "18th Oct", "28 Oct", "28/10", "28-10"
	dayMonthWordRegex = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?\s+(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\b`)
	monthDayWordRegex = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?\b`)
	dayOnlyRegex      = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)\b`)
	slashDateRegex    = regexp.MustCompile(`\b(\d{1,2})[/.-](\d{1,2})(?:[/.-](\d{2,4}))?\b`)

	// Weekdays
	weekdayMap = map[string]time.Weekday{
		"sunday":    time.Sunday,
		"sun":       time.Sunday,
		"ravivar":   time.Sunday,
		"ravi":      time.Sunday,
		"itwar":     time.Sunday,
		"monday":    time.Monday,
		"mon":       time.Monday,
		"somwar":    time.Monday,
		"som":       time.Monday,
		"tuesday":   time.Tuesday,
		"tue":       time.Tuesday,
		"mangalwar": time.Tuesday,
		"mangal":    time.Tuesday,
		"wednesday": time.Wednesday,
		"wed":       time.Wednesday,
		"budhwar":   time.Wednesday,
		"budh":      time.Wednesday,
		"thursday":  time.Thursday,
		"thu":       time.Thursday,
		"guruwar":   time.Thursday,
		"guru":      time.Thursday,
		"veervar":   time.Thursday,
		"friday":    time.Friday,
		"fri":       time.Friday,
		"shukrawar": time.Friday,
		"shukra":    time.Friday,
		"jumma":     time.Friday,
		"saturday":  time.Saturday,
		"sat":       time.Saturday,
		"shaniwar":  time.Saturday,
		"shani":     time.Saturday,
	}

	monthMap = map[string]time.Month{
		"jan": time.January, "january": time.January,
		"feb": time.February, "february": time.February,
		"mar": time.March, "march": time.March,
		"apr": time.April, "april": time.April,
		"may": time.May,
		"jun": time.June, "june": time.June,
		"jul": time.July, "july": time.July,
		"aug": time.August, "august": time.August,
		"sep": time.September, "september": time.September,
		"oct": time.October, "october": time.October,
		"nov": time.November, "november": time.November,
		"dec": time.December, "december": time.December,
	}

	// Time extraction: "5 pm", "11:59 pm", "17:00", "3 baje", "dopahar 2 baje"
	timeRegex = regexp.MustCompile(`(?i)\b(\d{1,2})(?::(\d{2}))?\s*(am|pm|baje)?\b`)
)

// ResolveDate resolves a raw text date and optional time expression relative to referenceTime
// contextSentence is passed to check for past vs future tense cues (e.g. for "kal")
func ResolveDate(rawDate, rawTime, contextSentence string, refTime time.Time) ResolutionResult {
	if refTime.IsZero() {
		refTime = time.Now()
	}
	loc := refTime.Location()

	dateExpr := strings.ToLower(strings.TrimSpace(rawDate))
	contextLower := strings.ToLower(contextSentence)

	res := ResolutionResult{
		Confidence: 0.90,
	}

	// 1. Check for relative words: aaj, kal, parso, tarso, today, tomorrow, yesterday
	refDateOnly := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 23, 59, 0, 0, loc)

	if strings.Contains(dateExpr, "aaj") || strings.Contains(dateExpr, "today") {
		res.ResolvedDate = refDateOnly
		res.Explanation = "Resolved 'aaj/today' to reference date"
	} else if strings.Contains(dateExpr, "parso") || strings.Contains(dateExpr, "day after tomorrow") {
		// Parso in future (day after tomorrow)
		res.ResolvedDate = refDateOnly.AddDate(0, 0, 2)
		res.Explanation = "Resolved 'parso' to 2 days after reference date"
	} else if strings.Contains(dateExpr, "tarso") {
		res.ResolvedDate = refDateOnly.AddDate(0, 0, 3)
		res.Explanation = "Resolved 'tarso' to 3 days after reference date"
	} else if strings.Contains(dateExpr, "kal") || strings.Contains(dateExpr, "tomorrow") || strings.Contains(dateExpr, "yesterday") {
		if strings.Contains(dateExpr, "yesterday") {
			res.ResolvedDate = refDateOnly.AddDate(0, 0, -1)
			res.Explanation = "Resolved 'yesterday' to 1 day before"
		} else if strings.Contains(dateExpr, "tomorrow") {
			res.ResolvedDate = refDateOnly.AddDate(0, 0, 1)
			res.Explanation = "Resolved 'tomorrow' to 1 day after"
		} else {
			// 'kal' in Hindi can mean yesterday or tomorrow depending on tense
			// Check tense cues: "hua tha", "gaya tha", "submit kiya", "tha", "the", "thi" vs "hoga", "karna hai", "hai", "submission", "due", "submit"
			pastTense := regexp.MustCompile(`\b(tha|the|thi|gaya|diya|hua|submited|submitted)\b`).MatchString(contextLower)
			futureTense := regexp.MustCompile(`\b(hoga|hogi|karna|karoge|dena|submit|due|tak|baje|subah|shaam)\b`).MatchString(contextLower)

			if pastTense {
				res.ResolvedDate = refDateOnly.AddDate(0, 0, -1)
				res.Explanation = "Resolved 'kal' to yesterday based on past tense cues"
				res.Confidence = 0.75
			} else if futureTense || strings.Contains(contextLower, "hai") {
				res.ResolvedDate = refDateOnly.AddDate(0, 0, 1)
				res.Explanation = "Resolved 'kal' to tomorrow based on future tense / deadline cues"
			} else {
				// Ambiguous: default to tomorrow for academic deadlines, but flag for review
				res.ResolvedDate = refDateOnly.AddDate(0, 0, 1)
				res.Confidence = 0.50
				res.NeedsReview = true
				res.Explanation = "'kal' is ambiguous between yesterday and tomorrow; flagged for review"
			}
		}
	} else if m := dayMonthWordRegex.FindStringSubmatch(dateExpr); m != nil {
		day, _ := strconv.Atoi(m[1])
		month := monthMap[m[2][:3]]
		year := refTime.Year()
		// If month is in past compared to ref, could be next year
		targetDate := time.Date(year, month, day, 23, 59, 0, 0, loc)
		if targetDate.Before(refDateOnly.AddDate(0, -6, 0)) {
			targetDate = targetDate.AddDate(1, 0, 0)
		}
		res.ResolvedDate = targetDate
		res.Explanation = fmt.Sprintf("Resolved '%s' to %04d-%02d-%02d", m[0], targetDate.Year(), targetDate.Month(), targetDate.Day())
	} else if m := monthDayWordRegex.FindStringSubmatch(dateExpr); m != nil {
		month := monthMap[m[1][:3]]
		day, _ := strconv.Atoi(m[2])
		year := refTime.Year()
		targetDate := time.Date(year, month, day, 23, 59, 0, 0, loc)
		res.ResolvedDate = targetDate
		res.Explanation = fmt.Sprintf("Resolved '%s' to %04d-%02d-%02d", m[0], targetDate.Year(), targetDate.Month(), targetDate.Day())
	} else if m := slashDateRegex.FindStringSubmatch(dateExpr); m != nil {
		d, _ := strconv.Atoi(m[1])
		mth, _ := strconv.Atoi(m[2])
		y := refTime.Year()
		if len(m) > 3 && m[3] != "" {
			yr, _ := strconv.Atoi(m[3])
			if yr < 100 {
				y = 2000 + yr
			} else {
				y = yr
			}
		}
		res.ResolvedDate = time.Date(y, time.Month(mth), d, 23, 59, 0, 0, loc)
		res.Explanation = fmt.Sprintf("Resolved slash date %s", m[0])
	} else if m := dayOnlyRegex.FindStringSubmatch(dateExpr); m != nil {
		day, _ := strconv.Atoi(m[1])
		targetDate := time.Date(refTime.Year(), refTime.Month(), day, 23, 59, 0, 0, loc)
		if targetDate.Before(refDateOnly.AddDate(0, 0, -5)) {
			// e.g. currently 28th and someone says "2nd", likely next month
			targetDate = targetDate.AddDate(0, 1, 0)
		}
		res.ResolvedDate = targetDate
		res.Explanation = fmt.Sprintf("Resolved '%sth' to %04d-%02d-%02d", m[1], targetDate.Year(), targetDate.Month(), targetDate.Day())
	} else {
		// Check for weekday names (e.g. "Friday", "next Monday tak", "agle somwar tak")
		foundWeekday := false
		for word, wd := range weekdayMap {
			// Match exact word boundary to prevent matching "mon" inside "monday" twice
			r := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
			if r.MatchString(dateExpr) {
				daysAhead := int(wd - refTime.Weekday())
				if daysAhead <= 0 {
					daysAhead += 7
				}
				res.ResolvedDate = refDateOnly.AddDate(0, 0, daysAhead)
				res.Explanation = fmt.Sprintf("Resolved weekday %s to +%d days", word, daysAhead)
				foundWeekday = true
				break
			}
		}

		if !foundWeekday {
			// Fallback: unable to resolve safely
			res.ResolvedDate = refDateOnly
			res.Confidence = 0.30
			res.NeedsReview = true
			res.Explanation = fmt.Sprintf("Could not deterministically resolve '%s'; routed to review queue", rawDate)
			return res
		}
	}

	// 2. Resolve explicit time if provided
	timeToUse := rawTime
	if timeToUse == "" {
		// Check if time expression is present in rawDate or context (e.g. "3 PM", "11:59 pm", "dopahar 3 baje")
		if m := regexp.MustCompile(`(?i)\b\d{1,2}(?::\d{2})?\s*(?:am|pm)\b`).FindString(dateExpr); m != "" {
			timeToUse = m
		}
	}

	if timeToUse != "" {
		hour, min, ok := parseTimeOfDay(timeToUse)
		if ok {
			res.ResolvedDate = time.Date(
				res.ResolvedDate.Year(),
				res.ResolvedDate.Month(),
				res.ResolvedDate.Day(),
				hour,
				min,
				0,
				0,
				loc,
			)
			res.HasExplicitTime = true
		}
	} else if strings.Contains(dateExpr, "eod") || strings.Contains(contextLower, "eod") {
		res.ResolvedDate = time.Date(
			res.ResolvedDate.Year(),
			res.ResolvedDate.Month(),
			res.ResolvedDate.Day(),
			23,
			59,
			0,
			0,
			loc,
		)
		res.HasExplicitTime = true
	}

	return res
}

func parseTimeOfDay(tStr string) (int, int, bool) {
	tStr = strings.ToLower(strings.TrimSpace(tStr))

	// Match "5 pm", "5:30 pm", "17:00", "3 baje"
	m := regexp.MustCompile(`(\d{1,2})(?::(\d{2}))?\s*(am|pm|baje)?`).FindStringSubmatch(tStr)
	if m == nil {
		return 0, 0, false
	}

	hour, _ := strconv.Atoi(m[1])
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
	}
	marker := m[3]

	if marker == "pm" && hour < 12 {
		hour += 12
	} else if marker == "am" && hour == 12 {
		hour = 0
	} else if marker == "baje" {
		// e.g. "shaam 5 baje", "dopahar 3 baje", "raat 10 baje"
		if strings.Contains(tStr, "shaam") || strings.Contains(tStr, "dopahar") || strings.Contains(tStr, "raat") {
			if hour < 12 {
				hour += 12
			}
		}
	}

	return hour, min, true
}
