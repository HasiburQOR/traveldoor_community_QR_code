package store

import (
	"database/sql"
	"errors"

	"traveldoor/qrprofile/internal/models"
)

const linkColumns = `id, profile_id, type, label, url, icon, sort_order, enabled`

func scanLink(row interface{ Scan(...any) error }) (*models.Link, error) {
	l := &models.Link{}
	err := row.Scan(&l.ID, &l.ProfileID, &l.Type, &l.Label, &l.URL, &l.Icon, &l.SortOrder, &l.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return l, err
}

func (s *Store) CreateLink(l *models.Link) (int64, error) {
	if l.SortOrder == 0 {
		var next sql.NullInt64
		if err := s.DB.QueryRow(`SELECT MAX(sort_order) FROM links WHERE profile_id = ?`, l.ProfileID).Scan(&next); err != nil {
			return 0, err
		}
		l.SortOrder = int(next.Int64) + 1
	}
	res, err := s.DB.Exec(`INSERT INTO links (profile_id, type, label, url, icon, sort_order, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ProfileID, l.Type, l.Label, l.URL, l.Icon, l.SortOrder, l.Enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateLink(l *models.Link) error {
	_, err := s.DB.Exec(`UPDATE links SET type = ?, label = ?, url = ?, icon = ?, enabled = ? WHERE id = ?`,
		l.Type, l.Label, l.URL, l.Icon, l.Enabled, l.ID)
	return err
}

func (s *Store) DeleteLink(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM links WHERE id = ?`, id)
	return err
}

func (s *Store) LinkByID(id int64) (*models.Link, error) {
	return scanLink(s.DB.QueryRow(`SELECT `+linkColumns+` FROM links WHERE id = ?`, id))
}

func (s *Store) ListLinks(profileID int64, onlyEnabled bool) ([]*models.Link, error) {
	q := `SELECT ` + linkColumns + ` FROM links WHERE profile_id = ?`
	if onlyEnabled {
		q += ` AND enabled = 1`
	}
	q += ` ORDER BY sort_order, id`

	rows, err := s.DB.Query(q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReorderLinks persists the given ordering, ignoring ids that do not belong to
// the profile.
func (s *Store) ReorderLinks(profileID int64, orderedIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE links SET sort_order = ? WHERE id = ? AND profile_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range orderedIDs {
		if _, err := stmt.Exec(i+1, id, profileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
