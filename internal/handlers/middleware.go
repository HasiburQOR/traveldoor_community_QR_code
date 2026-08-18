package handlers

import (
	"bytes"
	"net/http"
	"strings"
	"time"
)

// maxRequestBytes caps any authenticated mutation, including logo uploads.
const maxRequestBytes = 4 << 20

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "path", r.URL.Path, "error", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; "+
				"base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		if s.cfg.IsProduction() {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth guards admin routes and enforces CSRF on every mutation.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := s.auth.CurrentUser(r)
		if user == nil {
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", "/admin/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Cap the request body before anything parses it, so a large
			// upload cannot exhaust memory or disk.
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
			if !s.auth.CheckCSRF(r) {
				http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
				return
			}
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// renderPage executes a full page template into a buffer first, so a template
// error never produces a half-written response.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page string, status int, data map[string]any) {
	t, ok := s.pages[page]
	if !ok {
		s.log.Error("unknown template", "page", page)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["CSRFToken"] = s.auth.CSRFToken(w, r)
	data["BaseURL"] = s.cfg.BaseURL
	data["User"] = userFrom(r.Context())
	data["AssetVersion"] = s.assetVersion

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		s.log.Error("render", "page", page, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// renderPartial executes a named partial for an HTMX response.
func (s *Server) renderPartial(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["CSRFToken"] = s.auth.CSRFToken(w, r)

	var buf bytes.Buffer
	if err := s.pages["__partials__"].ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("render partial", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// refererHost keeps only the hostname of the referrer, so no full URL or query
// string is ever stored.
func refererHost(r *http.Request) string {
	ref := r.Referer()
	if ref == "" {
		return ""
	}
	ref = strings.TrimPrefix(strings.TrimPrefix(ref, "https://"), "http://")
	if i := strings.IndexAny(ref, "/?#"); i >= 0 {
		ref = ref[:i]
	}
	if len(ref) > 100 {
		ref = ref[:100]
	}
	return ref
}
