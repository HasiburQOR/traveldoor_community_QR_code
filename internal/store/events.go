package store

import (
	"database/sql"

	"traveldoor/qrprofile/internal/models"
)

const (
	EventPageView  = "page_view"
	EventLinkClick = "link_click"
)

// RecordEvent stores one aggregate-friendly analytics row. No IP address, user
// agent or cookie identifier is retained.
func (s *Store) RecordEvent(profileID int64, linkID *int64, eventType, referrerHost string) error {
	var lid any
	if linkID != nil {
		lid = *linkID
	}
	_, err := s.DB.Exec(`INSERT INTO events (profile_id, link_id, event_type, referrer) VALUES (?, ?, ?, ?)`,
		profileID, lid, eventType, referrerHost)
	return err
}

func (s *Store) ProfileStats(profileID int64) (*models.ProfileStats, error) {
	st := &models.ProfileStats{PerLink: map[int64]int{}}

	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM events WHERE profile_id = ? AND event_type = ?`,
		profileID, EventPageView).Scan(&st.Views); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM events WHERE profile_id = ? AND event_type = ?`,
		profileID, EventLinkClick).Scan(&st.Clicks); err != nil {
		return nil, err
	}

	rows, err := s.DB.Query(`SELECT link_id, COUNT(1) FROM events
		WHERE profile_id = ? AND event_type = ? AND link_id IS NOT NULL GROUP BY link_id`,
		profileID, EventLinkClick)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id sql.NullInt64
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		if id.Valid {
			st.PerLink[id.Int64] = n
		}
	}
	return st, rows.Err()
}
