package handlers

import (
	"net/http"
	"strconv"

	"traveldoor/qrprofile/internal/services"
)

// qrTarget is always the canonical public URL, so a printed QR keeps working
// after any content edit that leaves the slug unchanged.
func (s *Server) qrTarget(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	p := s.profileFromPath(w, r)
	if p == nil {
		return "", "", false
	}
	return s.PublicURL(p.Slug), p.Slug, true
}

func (s *Server) handleQRSVG(w http.ResponseWriter, r *http.Request) {
	target, slug, ok := s.qrTarget(w, r)
	if !ok {
		return
	}
	svg, err := services.QRSVG(target, 4)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`-qr.svg"`)
	}
	w.Write(svg)
}

func (s *Server) handleQRJPG(w http.ResponseWriter, r *http.Request) {
	target, slug, ok := s.qrTarget(w, r)
	if !ok {
		return
	}
	jpg, err := services.QRJPEG(target, qrSize(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`-qr.jpg"`)
	}
	w.Write(jpg)
}

func (s *Server) handleQRPDF(w http.ResponseWriter, r *http.Request) {
	target, slug, ok := s.qrTarget(w, r)
	if !ok {
		return
	}
	pdf, err := services.QRPDF(target, target)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+slug+`-qr.pdf"`)
	w.Write(pdf)
}

// qrSize reads the requested pixel size, clamped by the renderer.
func qrSize(r *http.Request) int {
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size == 0 {
		size = 1024
	}
	return size
}

func (s *Server) handleQRPNG(w http.ResponseWriter, r *http.Request) {
	target, slug, ok := s.qrTarget(w, r)
	if !ok {
		return
	}
	png, err := services.QRPNG(target, qrSize(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`-qr.png"`)
	}
	w.Write(png)
}
