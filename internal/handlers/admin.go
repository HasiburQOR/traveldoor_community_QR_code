package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/services"
	"traveldoor/qrprofile/internal/store"
)

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if s.auth.CurrentUser(r) != nil {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	s.renderPage(w, r, "login", http.StatusOK, map[string]any{"Title": "Sign in"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.auth.CheckCSRF(r) {
		http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
		return
	}
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	user, err := s.auth.Authenticate(email, password)
	if err != nil {
		s.log.Warn("failed login", "email", email)
		s.renderPage(w, r, "login", http.StatusUnauthorized, map[string]any{
			"Title": "Sign in",
			"Error": "Invalid email or password.",
			"Email": email,
		})
		return
	}
	if err := s.auth.StartSession(w, user.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.EndSession(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.store.ListProfiles()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	type row struct {
		*models.Profile
		Views  int
		Clicks int
	}
	rows := make([]row, 0, len(profiles))
	for _, p := range profiles {
		st, err := s.store.ProfileStats(p.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		rows = append(rows, row{Profile: p, Views: st.Views, Clicks: st.Clicks})
	}
	s.renderPage(w, r, "dashboard", http.StatusOK, map[string]any{
		"Title":    "Dashboard",
		"Profiles": rows,
	})
}

func (s *Server) handleProfileNew(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "profile_new", http.StatusOK, map[string]any{
		"Title":   "New profile",
		"Profile": &models.Profile{},
	})
}

// readProfileForm validates the submitted profile fields into p.
func readProfileForm(r *http.Request, p *models.Profile) []string {
	var errs []string

	p.Name = strings.TrimSpace(r.PostFormValue("name"))
	if p.Name == "" {
		errs = append(errs, "Name is required.")
	}

	p.Slug = services.NormalizeSlug(r.PostFormValue("slug"))
	if p.Slug == "" {
		p.Slug = services.NormalizeSlug(p.Name)
	}
	if err := services.ValidateSlug(p.Slug); err != nil {
		errs = append(errs, "Slug: "+err.Error()+".")
	}

	p.Subtitle = strings.TrimSpace(r.PostFormValue("subtitle"))
	p.Description = strings.TrimSpace(r.PostFormValue("description"))
	p.Address = strings.TrimSpace(r.PostFormValue("address"))
	p.ThemeJSON = strings.TrimSpace(r.PostFormValue("theme_json"))
	if p.ThemeJSON == "" {
		p.ThemeJSON = "{}"
	}

	if v := strings.TrimSpace(r.PostFormValue("phone")); v != "" {
		phone, err := services.NormalizePhone(v)
		if err != nil {
			errs = append(errs, "Phone: "+err.Error()+".")
		} else {
			p.Phone = phone
		}
	} else {
		p.Phone = ""
	}

	if v := strings.TrimSpace(r.PostFormValue("email")); v != "" {
		email, err := services.ValidateEmail(v)
		if err != nil {
			errs = append(errs, "Email: "+err.Error()+".")
		} else {
			p.Email = email
		}
	} else {
		p.Email = ""
	}

	if v := strings.TrimSpace(r.PostFormValue("website")); v != "" {
		site, err := services.ValidateURL(v)
		if err != nil {
			errs = append(errs, "Website: "+err.Error()+".")
		} else {
			p.Website = site
		}
	} else {
		p.Website = ""
	}

	return errs
}

func (s *Server) handleProfileCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p := &models.Profile{}
	errs := readProfileForm(r, p)

	if len(errs) == 0 {
		if taken, err := s.store.SlugTaken(p.Slug, 0); err != nil {
			s.serverError(w, r, err)
			return
		} else if taken {
			errs = append(errs, "Slug is already in use.")
		}
	}
	if len(errs) > 0 {
		s.renderPage(w, r, "profile_new", http.StatusUnprocessableEntity, map[string]any{
			"Title": "New profile", "Profile": p, "Errors": errs,
		})
		return
	}

	id, err := s.store.CreateProfile(p)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/profiles/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) profileFromPath(w http.ResponseWriter, r *http.Request) *models.Profile {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	p, err := s.store.ProfileByID(id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil
	}
	return p
}

func (s *Server) renderProfileEdit(w http.ResponseWriter, r *http.Request, p *models.Profile, status int, errs []string, notice string) {
	links, err := s.store.ListLinks(p.ID, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	stats, err := s.store.ProfileStats(p.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	publicURL := s.PublicURL(p.Slug)
	s.renderPage(w, r, "profile_edit", status, map[string]any{
		"Title":     "Edit " + p.Name,
		"QRVersion": qrFingerprint(publicURL),
		"Now":       time.Now().Unix(),
		"Profile":   p,
		"Links":     links,
		"LinkTypes": services.LinkTypes,
		"Stats":     stats,
		"PublicURL": publicURL,
		"Errors":    errs,
		"Notice":    notice,
	})
}

func (s *Server) handleProfileEdit(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	s.renderProfileEdit(w, r, p, http.StatusOK, nil, "")
}

func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	previousSlug := p.Slug
	errs := readProfileForm(r, p)

	if r.PostFormValue("remove_logo") == "1" {
		p.LogoPath = ""
	}
	if path, err := s.saveLogo(r, p); err != nil {
		errs = append(errs, "Logo: "+err.Error()+".")
	} else if path != "" {
		p.LogoPath = path
	}

	if len(errs) == 0 {
		if taken, err := s.store.SlugTaken(p.Slug, p.ID); err != nil {
			s.serverError(w, r, err)
			return
		} else if taken {
			errs = append(errs, "Slug is already in use.")
		}
	}
	if len(errs) > 0 {
		s.renderProfileEdit(w, r, p, http.StatusUnprocessableEntity, errs, "")
		return
	}
	if err := s.store.UpdateProfile(p); err != nil {
		s.serverError(w, r, err)
		return
	}
	notice := "Profile saved."
	if p.Slug != previousSlug {
		notice = "Profile saved. The slug changed, so any printed QR code for /" +
			previousSlug + " no longer resolves — regenerate and reprint it."
	}
	s.renderProfileEdit(w, r, p, http.StatusOK, nil, notice)
}

func (s *Server) handleProfilePublish(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	publish := r.PostFormValue("published") == "1"
	if err := s.store.SetProfilePublished(p.ID, publish); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/profiles/"+strconv.FormatInt(p.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	if err := s.store.DeleteProfile(p.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// qrFingerprint is a short, stable digest of whatever the QR encodes. It busts
// the preview cache and gives the admin a value to compare against a code
// already in circulation.
func qrFingerprint(target string) string {
	sum := sha256.Sum256([]byte(target))
	return hex.EncodeToString(sum[:])[:8]
}
