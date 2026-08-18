package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"traveldoor/qrprofile/internal/models"
)

// maxLogoBytes caps an uploaded logo. Anything larger is rejected outright.
const maxLogoBytes = 2 << 20 // 2 MiB

// allowedLogoTypes maps a sniffed content type to the extension we store.
// Only raster formats are accepted: an uploaded SVG would be script-capable
// when served from our own origin.
var allowedLogoTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// saveLogo stores an uploaded logo for the profile and returns the public path.
// It returns ("", nil) when the request carries no file.
func (s *Server) saveLogo(r *http.Request, p *models.Profile) (string, error) {
	file, header, err := r.FormFile("logo")
	if errors.Is(err, http.ErrMissingFile) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("logo upload could not be read")
	}
	defer file.Close()

	if header.Size > maxLogoBytes {
		return "", fmt.Errorf("logo must be %d KB or smaller", maxLogoBytes/1024)
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", errors.New("logo upload could not be read")
	}
	ext, ok := allowedLogoTypes[http.DetectContentType(head[:n])]
	if !ok {
		return "", errors.New("logo must be a PNG, JPEG, WebP or GIF image")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", errors.New("logo upload could not be read")
	}

	if err := os.MkdirAll(s.cfg.UploadDir, 0o755); err != nil {
		return "", err
	}
	// The filename is derived from the profile id, never from user input.
	name := "profile-" + strconv.FormatInt(p.ID, 10) + ext
	dest := filepath.Join(s.cfg.UploadDir, name)

	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(file, maxLogoBytes)); err != nil {
		return "", err
	}

	// A format change leaves the previous file behind; remove it.
	for _, other := range allowedLogoTypes {
		if other == ext {
			continue
		}
		os.Remove(filepath.Join(s.cfg.UploadDir, "profile-"+strconv.FormatInt(p.ID, 10)+other))
	}
	return "/uploads/" + name, nil
}

// handleUploadedFile serves stored logos. Only the flat upload directory is
// reachable and the filename is stripped of any path element.
func (s *Server) handleUploadedFile(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.TrimSpace(r.PathValue("file")))
	if name == "." || name == ".." || name == "/" || name == "" || strings.HasPrefix(name, ".") {
		s.notFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.UploadDir, name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		s.notFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path)
}
