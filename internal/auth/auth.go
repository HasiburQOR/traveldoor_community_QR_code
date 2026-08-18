// Package auth provides password hashing, cookie sessions and CSRF tokens for
// the admin area.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/store"
)

const (
	SessionCookie = "qrp_session"
	CSRFCookie    = "qrp_csrf"
	CSRFField     = "csrf_token"
	SessionTTL    = 12 * time.Hour
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type Manager struct {
	store  *store.Store
	secure bool
}

func New(s *store.Store, secure bool) *Manager {
	return &Manager{store: s, secure: secure}
}

func HashPassword(plain string) (string, error) {
	if len(plain) < 10 {
		return "", errors.New("password must be at least 10 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// Authenticate verifies credentials. The password comparison runs even for an
// unknown email so the response time does not leak account existence.
func (m *Manager) Authenticate(email, password string) (*models.User, error) {
	u, err := m.store.UserByEmail(email)
	if err != nil {
		bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalidin"), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// StartSession creates a server-side session and sets the session cookie.
func (m *Manager) StartSession(w http.ResponseWriter, userID int64) error {
	id, err := randomToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(SessionTTL)
	if err := m.store.CreateSession(id, userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    id,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (m *Manager) EndSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		_ = m.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode,
	})
}

// CurrentUser returns the signed-in user, or nil.
func (m *Manager) CurrentUser(r *http.Request) *models.User {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	sess, err := m.store.Session(c.Value)
	if err != nil {
		return nil
	}
	u, err := m.store.UserByID(sess.UserID)
	if err != nil {
		return nil
	}
	return u
}

// CSRFToken returns the token bound to this browser, creating and setting the
// cookie when it is missing. Mutations must echo it back in a form field or the
// X-CSRF-Token header (double-submit cookie pattern).
func (m *Manager) CSRFToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(CSRFCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	token, err := randomToken()
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // readable by HTMX for the request header
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

// CheckCSRF compares the submitted token with the cookie in constant time.
func (m *Manager) CheckCSRF(r *http.Request) bool {
	c, err := r.Cookie(CSRFCookie)
	if err != nil || c.Value == "" {
		return false
	}
	sent := r.Header.Get("X-CSRF-Token")
	if sent == "" {
		sent = r.PostFormValue(CSRFField)
	}
	if sent == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sent), []byte(c.Value)) == 1
}
