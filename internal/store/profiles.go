package store

import (
	"database/sql"
	"errors"

	"traveldoor/qrprofile/internal/models"
)

const profileColumns = `id, slug, name, subtitle, description, logo_path, theme_json,
	phone, email, address, website, published, created_at, updated_at`

func scanProfile(row interface{ Scan(...any) error }) (*models.Profile, error) {
	p := &models.Profile{}
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Subtitle, &p.Description, &p.LogoPath,
		&p.ThemeJSON, &p.Phone, &p.Email, &p.Address, &p.Website, &p.Published,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) CreateProfile(p *models.Profile) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO profiles
		(slug, name, subtitle, description, logo_path, theme_json, phone, email, address, website, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Slug, p.Name, p.Subtitle, p.Description, p.LogoPath, p.ThemeJSON,
		p.Phone, p.Email, p.Address, p.Website, p.Published)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateProfile(p *models.Profile) error {
	_, err := s.DB.Exec(`UPDATE profiles SET
		slug = ?, name = ?, subtitle = ?, description = ?, logo_path = ?, theme_json = ?,
		phone = ?, email = ?, address = ?, website = ?, updated_at = datetime('now')
		WHERE id = ?`,
		p.Slug, p.Name, p.Subtitle, p.Description, p.LogoPath, p.ThemeJSON,
		p.Phone, p.Email, p.Address, p.Website, p.ID)
	return err
}

func (s *Store) SetProfilePublished(id int64, published bool) error {
	_, err := s.DB.Exec(`UPDATE profiles SET published = ?, updated_at = datetime('now') WHERE id = ?`,
		published, id)
	return err
}

func (s *Store) DeleteProfile(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

func (s *Store) ProfileByID(id int64) (*models.Profile, error) {
	return scanProfile(s.DB.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id))
}

func (s *Store) ProfileBySlug(slug string) (*models.Profile, error) {
	return scanProfile(s.DB.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE slug = ?`, slug))
}

// PublishedProfileBySlug returns the profile only when it is published, so
// unpublished slugs are indistinguishable from missing ones publicly.
func (s *Store) PublishedProfileBySlug(slug string) (*models.Profile, error) {
	return scanProfile(s.DB.QueryRow(`SELECT `+profileColumns+
		` FROM profiles WHERE slug = ? AND published = 1`, slug))
}

func (s *Store) ListProfiles() ([]*models.Profile, error) {
	rows, err := s.DB.Query(`SELECT ` + profileColumns + ` FROM profiles ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SlugTaken reports whether slug belongs to a profile other than excludeID.
func (s *Store) SlugTaken(slug string, excludeID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM profiles WHERE slug = ? AND id <> ?`, slug, excludeID).Scan(&n)
	return n > 0, err
}
