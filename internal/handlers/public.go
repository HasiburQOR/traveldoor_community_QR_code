package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/services"
	"traveldoor/qrprofile/internal/store"
)

// publicLink is the view model for one rendered link.
type publicLink struct {
	ID    int64
	Type  string
	Label string
	Href  string
}

// href routes http(s) destinations through the click-tracking redirect and
// leaves tel:/mailto: as direct actions so they still work without JavaScript.
func href(l *models.Link) string {
	if strings.HasPrefix(l.URL, "http://") || strings.HasPrefix(l.URL, "https://") {
		return "/go/" + strconv.FormatInt(l.ID, 10)
	}
	return l.URL
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.store.ListProfiles()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	published := make([]*models.Profile, 0, len(profiles))
	for _, p := range profiles {
		if p.Published {
			published = append(published, p)
		}
	}
	if len(published) == 1 {
		http.Redirect(w, r, "/"+published[0].Slug, http.StatusFound)
		return
	}
	s.renderPage(w, r, "index", http.StatusOK, map[string]any{
		"Title":    "Profiles",
		"Profiles": published,
	})
}

// handlePublic dispatches the public URL space: /{slug} and
// /{slug}/contact.vcf. Anything else is a 404.
func (s *Server) handlePublic(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 1 && parts[0] != "":
		s.handlePublicProfile(w, r, strings.ToLower(parts[0]))
	case len(parts) == 2 && parts[1] == "contact.vcf":
		s.handleVCard(w, r, strings.ToLower(parts[0]))
	default:
		s.notFound(w, r)
	}
}

func (s *Server) handlePublicProfile(w http.ResponseWriter, r *http.Request, slug string) {
	p, err := s.store.PublishedProfileBySlug(slug)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	links, err := s.store.ListLinks(p.ID, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view := make([]publicLink, 0, len(links))
	for _, l := range links {
		view = append(view, publicLink{ID: l.ID, Type: l.Type, Label: l.Label, Href: href(l)})
	}

	if err := s.store.RecordEvent(p.ID, nil, store.EventPageView, refererHost(r)); err != nil {
		s.log.Warn("record page view", "error", err)
	}

	ogImage := ""
	if p.LogoPath != "" {
		ogImage = s.cfg.BaseURL + p.LogoPath
	}

	// Content changes frequently; allow a short shared cache window only.
	w.Header().Set("Cache-Control", "public, max-age=60")
	s.renderPage(w, r, "profile", http.StatusOK, map[string]any{
		"Title":     p.Name,
		"Profile":   p,
		"Links":     view,
		"PublicURL": s.PublicURL(p.Slug),
		"OGImage":   ogImage,
	})
}

func (s *Server) handleLinkRedirect(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	l, err := s.store.LinkByID(id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !l.Enabled) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	p, err := s.store.ProfileByID(l.ProfileID)
	if err != nil || !p.Published {
		s.notFound(w, r)
		return
	}
	// Re-validate at redirect time: never send a browser to a scheme that is
	// not on the allowlist, whatever is stored.
	dest, err := services.ValidateURL(l.URL)
	if err != nil {
		s.notFound(w, r)
		return
	}
	if err := s.store.RecordEvent(p.ID, &l.ID, store.EventLinkClick, refererHost(r)); err != nil {
		s.log.Warn("record click", "error", err)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, dest, http.StatusFound)
}

func (s *Server) handleVCard(w http.ResponseWriter, r *http.Request, slug string) {
	p, err := s.store.PublishedProfileBySlug(slug)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	links, err := s.store.ListLinks(p.ID, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	card := services.VCard(p, s.PublicURL(p.Slug), links)

	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.vcf"`, p.Slug))
	w.Write(card)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "notfound", http.StatusNotFound, map[string]any{
		"Title": "Page not available",
	})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("server error", "path", r.URL.Path, "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
