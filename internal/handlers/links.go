package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/services"
	"traveldoor/qrprofile/internal/store"
)

// respondLinks returns the refreshed link list for HTMX, or falls back to a
// normal redirect so the admin works without JavaScript.
func (s *Server) respondLinks(w http.ResponseWriter, r *http.Request, profileID int64, errMsg string) {
	if !isHTMX(r) {
		http.Redirect(w, r, "/admin/profiles/"+strconv.FormatInt(profileID, 10), http.StatusSeeOther)
		return
	}
	links, err := s.store.ListLinks(profileID, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderPartial(w, r, "link_list", map[string]any{
		"ProfileID": profileID,
		"Links":     links,
		"LinkTypes": services.LinkTypes,
		"Error":     errMsg,
	})
}

func (s *Server) handleLinkCreate(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	linkType := strings.TrimSpace(r.PostFormValue("type"))
	label := strings.TrimSpace(r.PostFormValue("label"))
	rawURL := r.PostFormValue("url")

	if !services.IsLinkType(linkType) {
		s.respondLinks(w, r, p.ID, "Unknown link type.")
		return
	}
	dest, err := services.NormalizeLinkURL(linkType, rawURL)
	if err != nil {
		s.respondLinks(w, r, p.ID, "Link: "+err.Error()+".")
		return
	}
	if label == "" {
		label = services.DefaultLabel(linkType)
	}

	l := &models.Link{
		ProfileID: p.ID,
		Type:      linkType,
		Label:     label,
		URL:       dest,
		Icon:      linkType,
		Enabled:   true,
	}
	if _, err := s.store.CreateLink(l); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondLinks(w, r, p.ID, "")
}

func (s *Server) linkFromPath(w http.ResponseWriter, r *http.Request) *models.Link {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	l, err := s.store.LinkByID(id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	if err != nil {
		s.serverError(w, r, err)
		return nil
	}
	return l
}

func (s *Server) handleLinkUpdate(w http.ResponseWriter, r *http.Request) {
	l := s.linkFromPath(w, r)
	if l == nil {
		return
	}
	// A bare toggle submits only the enabled field.
	if r.PostFormValue("toggle") == "1" {
		l.Enabled = !l.Enabled
		if err := s.store.UpdateLink(l); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.respondLinks(w, r, l.ProfileID, "")
		return
	}

	linkType := strings.TrimSpace(r.PostFormValue("type"))
	if !services.IsLinkType(linkType) {
		s.respondLinks(w, r, l.ProfileID, "Unknown link type.")
		return
	}
	dest, err := services.NormalizeLinkURL(linkType, r.PostFormValue("url"))
	if err != nil {
		s.respondLinks(w, r, l.ProfileID, "Link: "+err.Error()+".")
		return
	}
	label := strings.TrimSpace(r.PostFormValue("label"))
	if label == "" {
		label = services.DefaultLabel(linkType)
	}

	l.Type = linkType
	l.Label = label
	l.URL = dest
	l.Icon = linkType
	l.Enabled = r.PostFormValue("enabled") == "1"

	if err := s.store.UpdateLink(l); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondLinks(w, r, l.ProfileID, "")
}

func (s *Server) handleLinkDelete(w http.ResponseWriter, r *http.Request) {
	l := s.linkFromPath(w, r)
	if l == nil {
		return
	}
	if err := s.store.DeleteLink(l.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondLinks(w, r, l.ProfileID, "")
}

// handleLinkReorder accepts either an explicit "order" list (comma separated
// ids) or a single id plus a direction. The admin UI uses the second form via
// up/down buttons, so reordering works without JavaScript.
func (s *Server) handleLinkReorder(w http.ResponseWriter, r *http.Request) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return
	}
	links, err := s.store.ListLinks(p.ID, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	var ordered []int64
	if raw := strings.TrimSpace(r.PostFormValue("order")); raw != "" {
		valid := map[int64]bool{}
		for _, l := range links {
			valid[l.ID] = true
		}
		seen := map[int64]bool{}
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || !valid[id] || seen[id] {
				continue
			}
			seen[id] = true
			ordered = append(ordered, id)
		}
		// Any link not mentioned keeps its relative position at the end.
		for _, l := range links {
			if !seen[l.ID] {
				ordered = append(ordered, l.ID)
			}
		}
	} else {
		id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
		if err != nil {
			s.respondLinks(w, r, p.ID, "Unknown link.")
			return
		}
		dir := r.PostFormValue("direction")
		idx := -1
		for i, l := range links {
			ordered = append(ordered, l.ID)
			if l.ID == id {
				idx = i
			}
		}
		if idx < 0 {
			s.respondLinks(w, r, p.ID, "Unknown link.")
			return
		}
		swap := idx - 1
		if dir == "down" {
			swap = idx + 1
		}
		if swap >= 0 && swap < len(ordered) {
			ordered[idx], ordered[swap] = ordered[swap], ordered[idx]
		}
	}

	if err := s.store.ReorderLinks(p.ID, ordered); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondLinks(w, r, p.ID, "")
}
