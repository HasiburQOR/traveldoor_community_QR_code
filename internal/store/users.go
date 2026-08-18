package store

import (
	"database/sql"
	"errors"
	"strings"

	"traveldoor/qrprofile/internal/models"
)

var ErrNotFound = errors.New("not found")

func (s *Store) CreateUser(email, passwordHash string) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO users (email, password_hash) VALUES (?, ?)`,
		strings.ToLower(strings.TrimSpace(email)), passwordHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UserByEmail(email string) (*models.User, error) {
	u := &models.User{}
	err := s.DB.QueryRow(`SELECT id, email, password_hash, created_at FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByID(id int64) (*models.User, error) {
	u := &models.User{}
	err := s.DB.QueryRow(`SELECT id, email, password_hash, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) UpdateUserPassword(id int64, passwordHash string) error {
	_, err := s.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}
