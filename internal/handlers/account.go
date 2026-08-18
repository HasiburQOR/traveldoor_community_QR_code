package handlers

import (
	"net/http"
	"strings"

	"traveldoor/qrprofile/internal/auth"
)

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "account", http.StatusOK, map[string]any{"Title": "Account"})
}

// handlePasswordChange requires the current password, so a borrowed session
// cannot be used to lock the real owner out.
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if user == nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	current := r.PostFormValue("current_password")
	next := r.PostFormValue("new_password")
	confirm := r.PostFormValue("confirm_password")

	fail := func(msg string) {
		s.renderPage(w, r, "account", http.StatusUnprocessableEntity, map[string]any{
			"Title": "Account",
			"Error": msg,
		})
	}

	if _, err := s.auth.Authenticate(user.Email, current); err != nil {
		s.log.Warn("password change with wrong current password", "email", user.Email)
		fail("Your current password is not correct.")
		return
	}
	if strings.TrimSpace(next) != next || next == "" {
		fail("The new password cannot be empty or padded with spaces.")
		return
	}
	if next != confirm {
		fail("The new password and its confirmation do not match.")
		return
	}
	if next == current {
		fail("The new password must be different from the current one.")
		return
	}

	hash, err := auth.HashPassword(next)
	if err != nil {
		fail(err.Error() + ".")
		return
	}
	if err := s.store.UpdateUserPassword(user.ID, hash); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.log.Info("password changed", "email", user.Email)

	s.renderPage(w, r, "account", http.StatusOK, map[string]any{
		"Title":  "Account",
		"Notice": "Password updated.",
	})
}
