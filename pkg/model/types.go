package model

import "time"

// EventKind defines the classification of detected events.
type EventKind string

const (
	KindAssignment   EventKind = "assignment"
	KindExam         EventKind = "exam"
	KindFee          EventKind = "fee"
	KindEvent        EventKind = "event"
	KindNotice       EventKind = "notice"
	KindCancellation EventKind = "cancellation"
	KindPostponement EventKind = "postponement"
	KindOther        EventKind = "other"
)

// ItemStatus defines the lifecycle status of a deadline item.
type ItemStatus string

const (
	StatusActive    ItemStatus = "active"
	StatusUpdated   ItemStatus = "updated"
	StatusPostponed ItemStatus = "postponed"
	StatusCancelled ItemStatus = "cancelled"
	StatusDismissed ItemStatus = "dismissed"
	StatusConfirmed ItemStatus = "confirmed"
)

// Message represents a single parsed WhatsApp chat message.
type Message struct {
	ID        string    `json:"id"`         // SHA-256 hash (timestamp + sender + text)
	GroupID   string    `json:"group_id"`   // Tag for group/file origin
	Timestamp time.Time `json:"timestamp"`  // Original message time in context
	Sender    string    `json:"sender"`     // Sender name or phone
	Text      string    `json:"text"`       // Normalized message content
	RawText   string    `json:"raw_text"`   // Exact verbatim line(s)
	IsSystem  bool      `json:"is_system"`  // True for system notifications
	IsMedia   bool      `json:"is_media"`   // True for omitted media / stickers
	IsDeleted bool      `json:"is_deleted"` // True for deleted message notices
}

// RawExtractedEvent is the raw fact structure output by the LLM (Gemma).
// The model returns raw text expressions without computing dates.
type RawExtractedEvent struct {
	Kind               EventKind `json:"kind"`
	Title              string    `json:"title"`
	SubjectOrCourse    *string   `json:"subject_or_course"`
	RawDateExpression  string    `json:"raw_date_expression"`
	RawTimeExpression  *string   `json:"raw_time_expression"`
	ActionNeeded       *string   `json:"action_needed"`
	RefersToEarlier    *string   `json:"refers_to_earlier"`
	SourceMessageIDs   []string  `json:"source_message_ids"`
	Confidence         float64   `json:"confidence"` // 0.0 - 1.0
	ExtractionMode     string    `json:"extraction_mode,omitempty"` // "gemma" or "rule"
}

// DeadlineItem represents the canonical living state of a deadline.
type DeadlineItem struct {
	ID              string        `json:"id"`
	GroupID         string        `json:"group_id"`
	Kind            EventKind     `json:"kind"`
	Title           string        `json:"title"`
	SubjectOrCourse string        `json:"subject_or_course,omitempty"`
	DueDate         time.Time     `json:"due_date"`
	HasExplicitTime bool          `json:"has_explicit_time"`
	Status          ItemStatus    `json:"status"`
	Confidence      float64       `json:"confidence"`
	NeedsReview     bool          `json:"needs_review"`
	ActionNeeded    string        `json:"action_needed,omitempty"`
	ExtractionMode  string        `json:"extraction_mode,omitempty"` // "gemma" or "rule"
	Evidence        []ItemHistory `json:"evidence"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// ItemHistory documents an event update, extension, or reminder in an item's lifetime.
type ItemHistory struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"item_id"`
	MessageID      string    `json:"message_id"`
	Sender         string    `json:"sender"`
	Timestamp      time.Time `json:"timestamp"`
	MessageText    string    `json:"message_text"`
	ActionTaken    string    `json:"action_taken"` // "created", "extended", "postponed", "cancelled", "reminder"
	OldDueDate     *time.Time `json:"old_due_date,omitempty"`
	NewDueDate     *time.Time `json:"new_due_date,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}
