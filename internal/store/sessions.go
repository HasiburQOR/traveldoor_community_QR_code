package store

import (
	"database/sql"
	"errors"
	"time"

	"traveldoor/qrprofile/internal/models"
)

const sessionTimeLayout = time.RFC3339

func (s *Store) CreateSession(id string, userID int64, expires time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, expires.UTC().Format(sessionTimeLayout))
	return err
}

func (s *Store) Session(id string) (*models.Session, error) {
	var (
		sess    models.Session
		expires string
	)
	err := s.DB.QueryRow(`SELECT id, user_id, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(sessionTimeLayout, expires)
	if err != nil {
		return nil, err
	}
	sess.ExpiresAt = t
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(id)
		return nil, ErrNotFound
	}
	return &sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteExpiredSessions() error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(sessionTimeLayout))
	return err
}
