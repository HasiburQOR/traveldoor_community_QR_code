// Package handlers wires HTTP routes to the store and services.
package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	root "traveldoor/qrprofile"
	"traveldoor/qrprofile/internal/auth"
	"traveldoor/qrprofile/internal/config"
	"traveldoor/qrprofile/internal/store"
)

type Server struct {
	cfg    *config.Config
	store  *store.Store
	auth   *auth.Manager
	log    *slog.Logger
	pages  map[string]*template.Template
	static http.Handler
	// assetVersion busts the browser cache whenever the bundled CSS or JS
	// changes, so a redeploy is never served stale styling.
	assetVersion string
}

func New(cfg *config.Config, st *store.Store, log *slog.Logger) (*Server, error) {
	s := &Server{
		cfg:   cfg,
		store: st,
		auth:  auth.New(st, cfg.IsProduction()),
		log:   log,
	}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}

	staticFS, err := fs.Sub(root.StaticFS, "static")
	if err != nil {
		return nil, err
	}
	s.static = http.StripPrefix("/static/", cacheStatic(http.FileServer(http.FS(staticFS))))
	s.assetVersion = assetVersion(staticFS)
	return s, nil
}

// assetVersion hashes every bundled static file into a short fingerprint.
func assetVersion(staticFS fs.FS) string {
	h := sha256.New()
	err := fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(staticFS, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(body)
		return nil
	})
	if err != nil {
		// A fingerprint we cannot compute is better replaced by a value that
		// simply never caches than by a stale one.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

var templateFuncs = template.FuncMap{
	// safeURL allows the tel:/mailto: destinations that html/template would
	// otherwise filter; every stored URL has already passed the scheme
	// allowlist in services.ValidateURL.
	"safeURL": func(u string) template.URL { return template.URL(u) },
	"add":     func(a, b int) int { return a + b },
	"initial": func(s string) string {
		for _, r := range s {
			return strings.ToUpper(string(r))
		}
		return "?"
	},
	// dict builds a map inline so a partial can be invoked with an explicit
	// argument set rather than the whole page context.
	"dict": func(kv ...any) (map[string]any, error) {
		if len(kv)%2 != 0 {
			return nil, fmt.Errorf("dict requires an even number of arguments")
		}
		m := make(map[string]any, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			key, ok := kv[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			m[key] = kv[i+1]
		}
		return m, nil
	},
	"isExternal": func(u string) bool {
		return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
	},
}

// parseTemplates builds one template set per page so that every page can
// define its own "content" block against a shared layout.
func (s *Server) parseTemplates() error {
	s.pages = map[string]*template.Template{}

	sets := []struct {
		layout string
		glob   string
	}{
		{"templates/layouts/public.html", "templates/public/*.html"},
		{"templates/layouts/admin.html", "templates/admin/*.html"},
	}

	for _, set := range sets {
		files, err := fs.Glob(root.TemplateFS, set.glob)
		if err != nil {
			return err
		}
		for _, file := range files {
			name := strings.TrimSuffix(path.Base(file), ".html")
			t := template.New(path.Base(set.layout)).Funcs(templateFuncs)
			patterns := []string{set.layout, file, "templates/partials/*.html"}
			t, err := t.ParseFS(root.TemplateFS, patterns...)
			if err != nil {
				return fmt.Errorf("parse %s: %w", file, err)
			}
			s.pages[name] = t
		}
	}

	// Standalone partials, rendered directly for HTMX responses.
	partials := template.New("partials").Funcs(templateFuncs)
	partials, err := partials.ParseFS(root.TemplateFS, "templates/partials/*.html")
	if err != nil {
		return err
	}
	s.pages["__partials__"] = partials
	return nil
}

// Routes returns the fully wired handler, including global middleware.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /static/", s.static)
	mux.HandleFunc("GET /uploads/{file}", s.handleUploadedFile)

	// Admin
	mux.HandleFunc("GET /admin/login", s.handleLoginForm)
	mux.HandleFunc("POST /admin/login", s.handleLogin)
	mux.HandleFunc("POST /admin/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /admin", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("GET /admin/account", s.requireAuth(s.handleAccount))
	mux.HandleFunc("POST /admin/account/password", s.requireAuth(s.handlePasswordChange))
	mux.HandleFunc("GET /admin/profiles/new", s.requireAuth(s.handleProfileNew))
	mux.HandleFunc("POST /admin/profiles", s.requireAuth(s.handleProfileCreate))
	mux.HandleFunc("GET /admin/profiles/{id}", s.requireAuth(s.handleProfileEdit))
	mux.HandleFunc("POST /admin/profiles/{id}", s.requireAuth(s.handleProfileUpdate))
	mux.HandleFunc("POST /admin/profiles/{id}/publish", s.requireAuth(s.handleProfilePublish))
	mux.HandleFunc("POST /admin/profiles/{id}/delete", s.requireAuth(s.handleProfileDelete))
	mux.HandleFunc("POST /admin/profiles/{id}/links", s.requireAuth(s.handleLinkCreate))
	mux.HandleFunc("POST /admin/profiles/{id}/links/reorder", s.requireAuth(s.handleLinkReorder))
	mux.HandleFunc("POST /admin/links/{id}", s.requireAuth(s.handleLinkUpdate))
	mux.HandleFunc("POST /admin/links/{id}/delete", s.requireAuth(s.handleLinkDelete))
	mux.HandleFunc("GET /admin/profiles/{id}/qr.svg", s.requireAuth(s.handleQRSVG))
	mux.HandleFunc("GET /admin/profiles/{id}/qr.png", s.requireAuth(s.handleQRPNG))
	mux.HandleFunc("GET /admin/profiles/{id}/qr.jpg", s.requireAuth(s.handleQRJPG))
	mux.HandleFunc("GET /admin/profiles/{id}/qr.pdf", s.requireAuth(s.handleQRPDF))

	// Public. The profile routes are served from the catch-all so that
	// "/{slug}" and "/{slug}/contact.vcf" cannot conflict with the literal
	// prefixes above; ServeMux treats those overlaps as ambiguous.
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /go/{id}", s.handleLinkRedirect)
	mux.HandleFunc("GET /", s.handlePublic)

	return s.recoverPanic(s.securityHeaders(s.logRequests(mux)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.Ping(); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// PublicURL is the canonical, QR-encoded URL for a slug.
func (s *Server) PublicURL(slug string) string {
	return s.cfg.BaseURL + "/" + slug
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}
