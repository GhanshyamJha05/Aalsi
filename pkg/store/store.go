package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
	_ "modernc.org/sqlite"
)

// Store encapsulates SQLite database operations
type Store struct {
	db *sql.DB
}

// Open creates or connects to an SQLite database file and runs migrations
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db %s: %w", dbPath, err)
	}

	// Pragmas for reliability and concurrent read speed
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("executing pragma %s: %w", pragma, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	return s, nil
}

// Close closes the underlying database
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		group_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		sender TEXT NOT NULL,
		text TEXT NOT NULL,
		raw_text TEXT NOT NULL,
		is_system INTEGER NOT NULL,
		is_media INTEGER NOT NULL,
		is_deleted INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_messages_group_time ON messages(group_id, timestamp);

	CREATE TABLE IF NOT EXISTS deadline_items (
		id TEXT PRIMARY KEY,
		group_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		subject_or_course TEXT,
		due_date INTEGER NOT NULL,
		has_explicit_time INTEGER NOT NULL,
		status TEXT NOT NULL,
		confidence REAL NOT NULL,
		needs_review INTEGER NOT NULL,
		action_needed TEXT,
		extraction_mode TEXT DEFAULT 'rule',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_deadline_items_due ON deadline_items(due_date);
	CREATE INDEX IF NOT EXISTS idx_deadline_items_status ON deadline_items(status);

	CREATE TABLE IF NOT EXISTS item_history (
		id TEXT PRIMARY KEY,
		item_id TEXT NOT NULL,
		message_id TEXT NOT NULL,
		sender TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		message_text TEXT NOT NULL,
		action_taken TEXT NOT NULL,
		old_due_date INTEGER,
		new_due_date INTEGER,
		recorded_at INTEGER NOT NULL,
		FOREIGN KEY(item_id) REFERENCES deadline_items(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_item_history_item ON item_history(item_id);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Migrate existing database if column doesn't exist
	_, _ = s.db.Exec("ALTER TABLE deadline_items ADD COLUMN extraction_mode TEXT DEFAULT 'rule';")
	return nil
}

// ImportMessages stores parsed messages idempotently using INSERT OR IGNORE
// Returns count of newly inserted messages
func (s *Store) ImportMessages(ctx context.Context, messages []model.Message) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO messages (
			id, group_id, timestamp, sender, text, raw_text, is_system, is_media, is_deleted, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("preparing insert message: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	insertedCount := 0

	for _, m := range messages {
		res, err := stmt.ExecContext(ctx,
			m.ID,
			m.GroupID,
			m.Timestamp.Unix(),
			m.Sender,
			m.Text,
			m.RawText,
			boolToInt(m.IsSystem),
			boolToInt(m.IsMedia),
			boolToInt(m.IsDeleted),
			now,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting message %s: %w", m.ID, err)
		}
		rowsAffected, err := res.RowsAffected()
		if err == nil && rowsAffected > 0 {
			insertedCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing import transaction: %w", err)
	}

	return insertedCount, nil
}

// GetMessagesByGroup returns all non-system, non-media messages in chronological order for a group
func (s *Store) GetMessagesByGroup(ctx context.Context, groupID string) ([]model.Message, error) {
	query := `
		SELECT id, group_id, timestamp, sender, text, raw_text, is_system, is_media, is_deleted
		FROM messages
		WHERE group_id = ?
		ORDER BY timestamp ASC
	`
	rows, err := s.db.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Message
	for rows.Next() {
		var m model.Message
		var ts int64
		var isSys, isMed, isDel int
		err := rows.Scan(&m.ID, &m.GroupID, &ts, &m.Sender, &m.Text, &m.RawText, &isSys, &isMed, &isDel)
		if err != nil {
			return nil, err
		}
		m.Timestamp = time.Unix(ts, 0)
		m.IsSystem = isSys == 1
		m.IsMedia = isMed == 1
		m.IsDeleted = isDel == 1
		result = append(result, m)
	}
	return result, nil
}

// GetAllMessages returns all messages across groups ordered by timestamp
func (s *Store) GetAllMessages(ctx context.Context) ([]model.Message, error) {
	query := `
		SELECT id, group_id, timestamp, sender, text, raw_text, is_system, is_media, is_deleted
		FROM messages
		ORDER BY timestamp ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Message
	for rows.Next() {
		var m model.Message
		var ts int64
		var isSys, isMed, isDel int
		err := rows.Scan(&m.ID, &m.GroupID, &ts, &m.Sender, &m.Text, &m.RawText, &isSys, &isMed, &isDel)
		if err != nil {
			return nil, err
		}
		m.Timestamp = time.Unix(ts, 0)
		m.IsSystem = isSys == 1
		m.IsMedia = isMed == 1
		m.IsDeleted = isDel == 1
		result = append(result, m)
	}
	return result, nil
}

// SaveDeadlineItem creates or updates a deadline item and appends evidence
func (s *Store) SaveDeadlineItem(ctx context.Context, item *model.DeadlineItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	itemStmt := `
		INSERT INTO deadline_items (
			id, group_id, kind, title, subject_or_course, due_date, has_explicit_time,
			status, confidence, needs_review, action_needed, extraction_mode, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			title = excluded.title,
			subject_or_course = excluded.subject_or_course,
			due_date = excluded.due_date,
			has_explicit_time = excluded.has_explicit_time,
			status = excluded.status,
			confidence = excluded.confidence,
			needs_review = excluded.needs_review,
			action_needed = excluded.action_needed,
			extraction_mode = excluded.extraction_mode,
			updated_at = excluded.updated_at
	`
	extMode := item.ExtractionMode
	if extMode == "" {
		extMode = "rule"
	}

	_, err = tx.ExecContext(ctx, itemStmt,
		item.ID,
		item.GroupID,
		string(item.Kind),
		item.Title,
		item.SubjectOrCourse,
		item.DueDate.Unix(),
		boolToInt(item.HasExplicitTime),
		string(item.Status),
		item.Confidence,
		boolToInt(item.NeedsReview),
		item.ActionNeeded,
		extMode,
		item.CreatedAt.Unix(),
		item.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("upserting deadline item: %w", err)
	}

	histStmt := `
		INSERT OR IGNORE INTO item_history (
			id, item_id, message_id, sender, timestamp, message_text, action_taken,
			old_due_date, new_due_date, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	hPrepared, err := tx.PrepareContext(ctx, histStmt)
	if err != nil {
		return fmt.Errorf("preparing history insert: %w", err)
	}
	defer hPrepared.Close()

	for _, h := range item.Evidence {
		var oldUnix, newUnix *int64
		if h.OldDueDate != nil {
			v := h.OldDueDate.Unix()
			oldUnix = &v
		}
		if h.NewDueDate != nil {
			v := h.NewDueDate.Unix()
			newUnix = &v
		}

		_, err := hPrepared.ExecContext(ctx,
			h.ID,
			item.ID,
			h.MessageID,
			h.Sender,
			h.Timestamp.Unix(),
			h.MessageText,
			h.ActionTaken,
			oldUnix,
			newUnix,
			h.RecordedAt.Unix(),
		)
		if err != nil {
			return fmt.Errorf("inserting history entry: %w", err)
		}
	}

	return tx.Commit()
}

// GetItems returns all active/updated/postponed items
func (s *Store) GetItems(ctx context.Context, includeDismissed bool) ([]model.DeadlineItem, error) {
	query := `
		SELECT id, group_id, kind, title, subject_or_course, due_date, has_explicit_time,
		       status, confidence, needs_review, action_needed, COALESCE(extraction_mode, 'rule'), created_at, updated_at
		FROM deadline_items
	`
	if !includeDismissed {
		query += " WHERE status != 'dismissed'"
	}
	query += " ORDER BY due_date ASC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.DeadlineItem
	for rows.Next() {
		var it model.DeadlineItem
		var kind, status, subject, action, extMode string
		var dueUnix, createdUnix, updatedUnix int64
		var hasExpTime, needsRev int

		err := rows.Scan(
			&it.ID, &it.GroupID, &kind, &it.Title, &subject, &dueUnix, &hasExpTime,
			&status, &it.Confidence, &needsRev, &action, &extMode, &createdUnix, &updatedUnix,
		)
		if err != nil {
			return nil, err
		}

		it.Kind = model.EventKind(kind)
		it.Status = model.ItemStatus(status)
		it.SubjectOrCourse = subject
		it.ActionNeeded = action
		it.ExtractionMode = extMode
		it.DueDate = time.Unix(dueUnix, 0)
		it.CreatedAt = time.Unix(createdUnix, 0)
		it.UpdatedAt = time.Unix(updatedUnix, 0)
		it.HasExplicitTime = hasExpTime == 1
		it.NeedsReview = needsRev == 1

		items = append(items, it)
	}

	// Load evidence for each item
	for i := range items {
		ev, err := s.GetItemHistory(ctx, items[i].ID)
		if err == nil {
			items[i].Evidence = ev
		}
	}

	return items, nil
}

// GetItemHistory retrieves chronological history evidence for an item
func (s *Store) GetItemHistory(ctx context.Context, itemID string) ([]model.ItemHistory, error) {
	query := `
		SELECT id, item_id, message_id, sender, timestamp, message_text, action_taken,
		       old_due_date, new_due_date, recorded_at
		FROM item_history
		WHERE item_id = ?
		ORDER BY recorded_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.ItemHistory
	for rows.Next() {
		var h model.ItemHistory
		var ts, recorded int64
		var oldUnix, newUnix *int64

		err := rows.Scan(
			&h.ID, &h.ItemID, &h.MessageID, &h.Sender, &ts, &h.MessageText, &h.ActionTaken,
			&oldUnix, &newUnix, &recorded,
		)
		if err != nil {
			return nil, err
		}

		h.Timestamp = time.Unix(ts, 0)
		h.RecordedAt = time.Unix(recorded, 0)
		if oldUnix != nil {
			t := time.Unix(*oldUnix, 0)
			h.OldDueDate = &t
		}
		if newUnix != nil {
			t := time.Unix(*newUnix, 0)
			h.NewDueDate = &t
		}
		history = append(history, h)
	}
	return history, nil
}

// UpdateItemStatus updates the status and needs_review flag of an item
func (s *Store) UpdateItemStatus(ctx context.Context, itemID string, status model.ItemStatus, needsReview bool) error {
	query := `
		UPDATE deadline_items
		SET status = ?, needs_review = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query, string(status), boolToInt(needsReview), time.Now().Unix(), itemID)
	return err
}

// PurgeAll removes all data from tables
func (s *Store) PurgeAll(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM item_history"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM deadline_items"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM messages"); err != nil {
		return err
	}

	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
