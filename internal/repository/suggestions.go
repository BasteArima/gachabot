package repository

import (
	"database/sql"
	"time"
)

// Suggestion statuses.
const (
	SuggestionPending  = "pending"
	SuggestionApproved = "approved"
	SuggestionRejected = "rejected"
)

// Suggestion is one player-submitted card proposal.
type Suggestion struct {
	ID         int64      `json:"id"`
	UserID     *int64     `json:"userId"`
	AuthorName string     `json:"authorName"`
	Platform   string     `json:"platform"`
	Caption    string     `json:"caption"`
	FileID     string     `json:"fileId"`
	ImageURL   string     `json:"imageUrl"`
	Status     string     `json:"status"`
	ReviewNote string     `json:"reviewNote"`
	CardID     *int       `json:"cardId"`
	Refunded   bool       `json:"refunded"`
	CreatedAt  *time.Time `json:"createdAt"`
	ReviewedAt *time.Time `json:"reviewedAt"`
}

// CreateSuggestion records a submission. Called from the bots right after the
// player is charged, so a paid-for suggestion always leaves a trace.
func (r *PostgresRepo) CreateSuggestion(userID int64, platform, caption, fileID, imageURL string) (int64, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO suggestions (user_id, platform, caption, file_id, image_url)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')) RETURNING id`,
		userID, platform, caption, fileID, imageURL).Scan(&id)
	return id, err
}

// ListSuggestions returns suggestions, optionally filtered by status.
func (r *PostgresRepo) ListSuggestions(status string, limit int) ([]Suggestion, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(`
		SELECT s.id, s.user_id,
		       COALESCE(NULLIF(u.first_name, ''), NULLIF(u.username, ''), ''),
		       s.platform, s.caption, COALESCE(s.file_id, ''), COALESCE(s.image_url, ''),
		       s.status, COALESCE(s.review_note, ''), s.card_id, s.refunded,
		       s.created_at, s.reviewed_at
		FROM suggestions s
		LEFT JOIN users u ON u.id = s.user_id
		WHERE ($1 = '' OR s.status = $1)
		ORDER BY s.created_at DESC
		LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Suggestion, 0)
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSuggestion loads one suggestion.
func (r *PostgresRepo) GetSuggestion(id int64) (*Suggestion, error) {
	s, err := scanSuggestion(r.db.QueryRow(`
		SELECT s.id, s.user_id,
		       COALESCE(NULLIF(u.first_name, ''), NULLIF(u.username, ''), ''),
		       s.platform, s.caption, COALESCE(s.file_id, ''), COALESCE(s.image_url, ''),
		       s.status, COALESCE(s.review_note, ''), s.card_id, s.refunded,
		       s.created_at, s.reviewed_at
		FROM suggestions s
		LEFT JOIN users u ON u.id = s.user_id
		WHERE s.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ReviewSuggestion closes a suggestion as approved or rejected.
func (r *PostgresRepo) ReviewSuggestion(id int64, status, note string, cardID *int, refunded bool) error {
	res, err := r.db.Exec(`
		UPDATE suggestions
		SET status = $2, review_note = NULLIF($3, ''), card_id = $4, refunded = $5, reviewed_at = NOW()
		WHERE id = $1 AND status = 'pending'`,
		id, status, note, cardID, refunded)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CountPendingSuggestions powers the admin badge / overview.
func (r *PostgresRepo) CountPendingSuggestions() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM suggestions WHERE status = 'pending'`).Scan(&n)
	return n, err
}

func scanSuggestion(row interface{ Scan(...any) error }) (Suggestion, error) {
	var (
		s        Suggestion
		userID   sql.NullInt64
		cardID   sql.NullInt64
		created  sql.NullTime
		reviewed sql.NullTime
	)
	err := row.Scan(&s.ID, &userID, &s.AuthorName, &s.Platform, &s.Caption, &s.FileID, &s.ImageURL,
		&s.Status, &s.ReviewNote, &cardID, &s.Refunded, &created, &reviewed)
	if err != nil {
		return s, err
	}
	if userID.Valid {
		s.UserID = &userID.Int64
	}
	if cardID.Valid {
		v := int(cardID.Int64)
		s.CardID = &v
	}
	if created.Valid {
		s.CreatedAt = &created.Time
	}
	if reviewed.Valid {
		s.ReviewedAt = &reviewed.Time
	}
	return s, nil
}
