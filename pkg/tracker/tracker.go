package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/resolver"
	"github.com/aalsi-app/aalsi/pkg/store"
)

// Engine manages the state lifecycle of living deadline items
type Engine struct {
	store *store.Store
}

// New creates a new state tracking Engine
func New(s *store.Store) *Engine {
	return &Engine{store: s}
}

// ProcessRawEvent applies an incoming raw extracted event against existing items in SQLite
func (e *Engine) ProcessRawEvent(ctx context.Context, groupID string, raw model.RawExtractedEvent, msgMap map[string]model.Message) (*model.DeadlineItem, error) {
	// Find primary source message to establish reference time
	var refMsg model.Message
	if len(raw.SourceMessageIDs) > 0 {
		if m, ok := msgMap[raw.SourceMessageIDs[0]]; ok {
			refMsg = m
		}
	}
	if refMsg.Timestamp.IsZero() {
		refMsg.Timestamp = time.Now()
	}

	rawTimeStr := ""
	if raw.RawTimeExpression != nil {
		rawTimeStr = *raw.RawTimeExpression
	}

	subjectStr := ""
	if raw.SubjectOrCourse != nil {
		subjectStr = *raw.SubjectOrCourse
	}

	actionStr := ""
	if raw.ActionNeeded != nil {
		actionStr = *raw.ActionNeeded
	}

	// 1. Resolve calendar date deterministically in Go
	res := resolver.ResolveDate(raw.RawDateExpression, rawTimeStr, refMsg.Text, refMsg.Timestamp)

	// Combine model confidence with resolver confidence
	finalConfidence := raw.Confidence * res.Confidence

	// Requirement:
	// - An exam must never default to "EOD" (it requires an explicit time or must go to review queue)
	// - Only assignments and exams require a subject to skip review. Fees, notices, and events do not.
	// - Confidence below 0.7 goes to Review Queue.
	isExam := raw.Kind == model.KindExam || strings.Contains(strings.ToLower(raw.Title), "exam") || strings.Contains(strings.ToLower(raw.Title), "quiz")
	isAssignment := raw.Kind == model.KindAssignment || strings.Contains(strings.ToLower(raw.Title), "assignment") || strings.Contains(strings.ToLower(raw.Title), "lab")
	isEODOrUnresolved := !res.HasExplicitTime || (res.ResolvedDate.Hour() == 23 && res.ResolvedDate.Minute() == 59)
	hasNoSubject := strings.TrimSpace(subjectStr) == ""

	needsReview := res.NeedsReview || (finalConfidence < 0.70)
	// Only assignments and exams require a subject to skip review
	if (isExam || isAssignment) && hasNoSubject {
		needsReview = true
	}
	// An exam must never default to EOD
	if isExam && isEODOrUnresolved {
		needsReview = true
	}

	// 2. Fetch existing items for this group
	existingItems, err := e.store.GetItems(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("getting existing items: %w", err)
	}

	// 3. Match against existing items
	matchedItem, score := findBestMatch(raw, subjectStr, res.ResolvedDate, existingItems)

	now := time.Now()

	historyEntry := model.ItemHistory{
		ID:          generateID("hist", refMsg.ID+raw.Title+fmt.Sprintf("%d", now.UnixNano())),
		MessageID:   refMsg.ID,
		Sender:      refMsg.Sender,
		Timestamp:   refMsg.Timestamp,
		MessageText: refMsg.Text,
		RecordedAt:  now,
	}

	// Detect if this is an extension, postponement, or cancellation
	isCancel := raw.Kind == model.KindCancellation || strings.Contains(strings.ToLower(refMsg.Text), "cancel")
	isShiftOrExtend := raw.Kind == model.KindPostponement ||
		strings.Contains(strings.ToLower(refMsg.Text), "extend") ||
		strings.Contains(strings.ToLower(refMsg.Text), "postpone") ||
		strings.Contains(strings.ToLower(refMsg.Text), "shift") ||
		strings.Contains(strings.ToLower(refMsg.Text), "reschedule")

	if matchedItem != nil && score >= 0.70 {
		// Existing item found: mutate its state or merge reminder!
		oldDue := matchedItem.DueDate
		historyEntry.OldDueDate = &oldDue
		historyEntry.ItemID = matchedItem.ID

		if isCancel {
			matchedItem.Status = model.StatusCancelled
			matchedItem.UpdatedAt = now
			historyEntry.ActionTaken = "cancelled"
		} else if isShiftOrExtend || !datesMatch(oldDue, res.ResolvedDate) {
			if isShiftOrExtend {
				matchedItem.Status = model.StatusPostponed
				historyEntry.ActionTaken = "postponed"
			} else {
				matchedItem.Status = model.StatusUpdated
				historyEntry.ActionTaken = "updated"
			}
			matchedItem.DueDate = res.ResolvedDate
			matchedItem.HasExplicitTime = res.HasExplicitTime
			matchedItem.UpdatedAt = now
			historyEntry.NewDueDate = &res.ResolvedDate
		} else {
			// Same deadline date/time reiterated -> Merge reminder!
			historyEntry.ActionTaken = "reminder"
			matchedItem.UpdatedAt = now
		}

		if actionStr != "" && matchedItem.ActionNeeded == "" {
			matchedItem.ActionNeeded = actionStr
		}
		if subjectStr != "" && matchedItem.SubjectOrCourse == "" {
			matchedItem.SubjectOrCourse = subjectStr
		}
		if raw.ExtractionMode != "" {
			matchedItem.ExtractionMode = raw.ExtractionMode
		}

		matchedItem.Evidence = append(matchedItem.Evidence, historyEntry)
		if err := e.store.SaveDeadlineItem(ctx, matchedItem); err != nil {
			return nil, fmt.Errorf("updating matched item: %w", err)
		}
		return matchedItem, nil
	}

	// No existing item matched with high similarity: Create new item
	newItemID := generateID("item", groupID+subjectStr+raw.Title+res.ResolvedDate.Format("20060102"))
	historyEntry.ItemID = newItemID
	historyEntry.ActionTaken = "created"
	historyEntry.NewDueDate = &res.ResolvedDate

	status := model.StatusActive
	if isCancel {
		status = model.StatusCancelled
	}

	// Requirement 2: A postponement/extension/cancellation must never create a new item titled with the event kind
	cleanTitle := strings.TrimSpace(raw.Title)
	for _, forbidden := range []string{"cancellation", "postponement", "cancelled", "postponed"} {
		if strings.EqualFold(cleanTitle, forbidden) {
			if subjectStr != "" {
				cleanTitle = fmt.Sprintf("%s Update", subjectStr)
			} else {
				cleanTitle = "Class Notice"
			}
			break
		}
	}

	extMode := raw.ExtractionMode
	if extMode == "" {
		extMode = "rule"
	}

	newItem := &model.DeadlineItem{
		ID:              newItemID,
		GroupID:         groupID,
		Kind:            raw.Kind,
		Title:           cleanTitle,
		SubjectOrCourse: subjectStr,
		DueDate:         res.ResolvedDate,
		HasExplicitTime: res.HasExplicitTime,
		Status:          status,
		Confidence:      finalConfidence,
		NeedsReview:     needsReview,
		ActionNeeded:    actionStr,
		ExtractionMode:  extMode,
		CreatedAt:       now,
		UpdatedAt:       now,
		Evidence:        []model.ItemHistory{historyEntry},
	}

	if err := e.store.SaveDeadlineItem(ctx, newItem); err != nil {
		return nil, fmt.Errorf("saving new deadline item: %w", err)
	}

	return newItem, nil
}

// findBestMatch computes similarity against existing items
func findBestMatch(raw model.RawExtractedEvent, subject string, resolvedDate time.Time, existing []model.DeadlineItem) (*model.DeadlineItem, float64) {
	var bestItem *model.DeadlineItem
	bestScore := 0.0

	for i := range existing {
		item := &existing[i]
		score := computeMatchScore(raw, subject, resolvedDate, item)
		if score > bestScore {
			bestScore = score
			bestItem = item
		}
	}

	return bestItem, bestScore
}

func computeMatchScore(raw model.RawExtractedEvent, subject string, newDue time.Time, existing *model.DeadlineItem) float64 {
	// Rule: If subjects are explicitly different (e.g. "Maths" vs "OS"), score must NOT merge
	cleanSub1 := cleanSubject(subject)
	cleanSub2 := cleanSubject(existing.SubjectOrCourse)
	if cleanSub1 != "" && cleanSub2 != "" && cleanSub1 != cleanSub2 {
		return 0.0
	}

	// Compute title similarity
	titleSim := stringSimilarity(raw.Title, existing.Title)

	// Check if this refers to earlier item
	refersSim := 0.0
	if raw.RefersToEarlier != nil && *raw.RefersToEarlier != "" {
		refersSim = stringSimilarity(*raw.RefersToEarlier, existing.Title)
	}

	maxTitleSim := titleSim
	if refersSim > maxTitleSim {
		maxTitleSim = refersSim
	}

	// Distinct assignment test: e.g. "Assignment 1" vs "Assignment 2"
	// Extract trailing numbers in titles
	num1 := extractTrailingNumber(raw.Title)
	num2 := extractTrailingNumber(existing.Title)
	if num1 > 0 && num2 > 0 && num1 != num2 {
		return 0.0 // MUST NOT MERGE different assignments
	}

	// Temporal proximity score
	dayDiff := math.Abs(newDue.Sub(existing.DueDate).Hours() / 24.0)
	timeScore := 0.0
	if dayDiff < 1.0 {
		timeScore = 0.40 // exact or same-day
	} else if dayDiff <= 7.0 {
		timeScore = 0.25 // within one week extension
	} else if dayDiff <= 14.0 {
		timeScore = 0.10
	}

	// If this is a cancellation or postponement/extension, give extra weight to subject match and common activity keywords
	isUpdateEvent := raw.Kind == model.KindCancellation || raw.Kind == model.KindPostponement
	activityMatch := false
	lowerTitle := strings.ToLower(raw.Title)
	lowerRefers := ""
	if raw.RefersToEarlier != nil {
		lowerRefers = strings.ToLower(*raw.RefersToEarlier)
	}
	lowerExist := strings.ToLower(existing.Title)

	for _, act := range []string{"quiz", "viva", "lab", "assignment", "class", "test", "exam", "fee"} {
		if (strings.Contains(lowerTitle, act) || strings.Contains(lowerRefers, act)) && strings.Contains(lowerExist, act) {
			activityMatch = true
			break
		}
	}

	subjectMatchBonus := 0.0
	if cleanSub1 != "" && cleanSub1 == cleanSub2 {
		if isUpdateEvent && activityMatch {
			subjectMatchBonus = 0.65 // strongly link to the existing course item of same activity
		} else if maxTitleSim > 0.3 {
			subjectMatchBonus = 0.35
		}
	}

	totalScore := (maxTitleSim * 0.45) + timeScore + subjectMatchBonus
	return math.Min(1.0, totalScore)
}

func datesMatch(d1, d2 time.Time) bool {
	return d1.Year() == d2.Year() && d1.Month() == d2.Month() && d1.Day() == d2.Day() &&
		d1.Hour() == d2.Hour() && d1.Minute() == d2.Minute()
}

func cleanSubject(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "course", "")
	s = strings.ReplaceAll(s, "subject", "")
	return strings.TrimSpace(s)
}

func extractTrailingNumber(s string) int {
	re := regexp.MustCompile(`(?i)\b(?:assignment|lab|quiz|project|module|unit|hw|homework)\s*#?(\d+)\b`)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		return n
	}
	return 0
}

// Jaccard word n-gram similarity
func stringSimilarity(s1, s2 string) float64 {
	w1 := tokenize(s1)
	w2 := tokenize(s2)

	if len(w1) == 0 || len(w2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, w := range w1 {
		set1[w] = true
	}

	intersection := 0
	set2 := make(map[string]bool)
	for _, w := range w2 {
		set2[w] = true
		if set1[w] {
			intersection++
		}
	}

	union := len(set1)
	for w := range set2 {
		if !set1[w] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var res []string
	stopwords := map[string]bool{"due": true, "the": true, "is": true, "on": true, "at": true, "by": true, "for": true}
	for _, w := range words {
		w = strings.Trim(w, ",.!:;\"'()[]{}")
		if w != "" && !stopwords[w] {
			res = append(res, w)
		}
	}
	return res
}

func generateID(prefix, input string) string {
	h := sha256.New()
	h.Write([]byte(input))
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(h.Sum(nil))[:16])
}
