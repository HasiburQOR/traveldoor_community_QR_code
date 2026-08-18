// Package seed imports profile definitions from JSON, so a real profile can be
// created reproducibly without clicking through the admin UI.
package seed

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/services"
	"traveldoor/qrprofile/internal/store"
)

//go:embed data
var dataFS embed.FS

type linkSpec struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type profileSpec struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Website     string `json:"website"`
	Address     string `json:"address"`
	LogoPath    string `json:"logo_path"`
	// LogoFile names a bundled image under data/ that is copied into the
	// upload directory on import.
	LogoFile  string     `json:"logo_file"`
	ThemeJSON string     `json:"theme_json"`
	Published bool       `json:"published"`
	Links     []linkSpec `json:"links"`
}

// ImportDefaultIfMissing imports the bundled profile only when its slug does
// not exist yet. A full import replaces the profile's whole link set, which
// would discard edits made in the admin, so this is the safe form to run on
// every start of a hosted deployment.
func ImportDefaultIfMissing(st *store.Store, uploadDir string, log *slog.Logger) error {
	var spec profileSpec
	body, err := dataFS.ReadFile("data/traveldoor.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &spec); err != nil {
		return err
	}
	if _, err := st.ProfileBySlug(services.NormalizeSlug(spec.Slug)); err == nil {
		log.Info("seed: profile already exists, skipping", "slug", spec.Slug)
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return importJSON(st, body, uploadDir, log)
}

// ImportDefault imports the bundled Traveldoor profile, copying its bundled
// logo into uploadDir. It replaces the existing link set.
func ImportDefault(st *store.Store, uploadDir string, log *slog.Logger) error {
	body, err := dataFS.ReadFile("data/traveldoor.json")
	if err != nil {
		return err
	}
	return importJSON(st, body, uploadDir, log)
}

// ImportFile imports a profile definition from a JSON file on disk.
func ImportFile(st *store.Store, path, uploadDir string, log *slog.Logger) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return importJSON(st, body, uploadDir, log)
}

// importJSON upserts the profile by slug and replaces its link set. Re-running
// it is safe and leaves the slug — and therefore any printed QR — unchanged.
func importJSON(st *store.Store, body []byte, uploadDir string, log *slog.Logger) error {
	var spec profileSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return fmt.Errorf("parse seed: %w", err)
	}

	spec.Slug = services.NormalizeSlug(spec.Slug)
	if err := services.ValidateSlug(spec.Slug); err != nil {
		return fmt.Errorf("seed slug: %w", err)
	}
	if spec.Name == "" {
		return errors.New("seed profile requires a name")
	}
	if spec.ThemeJSON == "" {
		spec.ThemeJSON = "{}"
	}

	p := &models.Profile{
		Slug:        spec.Slug,
		Name:        spec.Name,
		Subtitle:    spec.Subtitle,
		Description: spec.Description,
		LogoPath:    spec.LogoPath,
		ThemeJSON:   spec.ThemeJSON,
		Address:     spec.Address,
		Published:   spec.Published,
	}
	if spec.Phone != "" {
		v, err := services.NormalizePhone(spec.Phone)
		if err != nil {
			return fmt.Errorf("seed phone: %w", err)
		}
		p.Phone = v
	}
	if spec.Email != "" {
		v, err := services.ValidateEmail(spec.Email)
		if err != nil {
			return fmt.Errorf("seed email: %w", err)
		}
		p.Email = v
	}
	if spec.Website != "" {
		v, err := services.ValidateURL(spec.Website)
		if err != nil {
			return fmt.Errorf("seed website: %w", err)
		}
		p.Website = v
	}

	existing, err := st.ProfileBySlug(spec.Slug)
	switch {
	case err == nil:
		p.ID = existing.ID
		// Never clobber branding uploaded through the admin: only a seed that
		// explicitly carries a logo replaces what is already there.
		if spec.LogoPath == "" && spec.LogoFile == "" {
			p.LogoPath = existing.LogoPath
		}
		if err := st.UpdateProfile(p); err != nil {
			return err
		}
		if err := st.SetProfilePublished(p.ID, spec.Published); err != nil {
			return err
		}
		links, err := st.ListLinks(p.ID, false)
		if err != nil {
			return err
		}
		for _, l := range links {
			if err := st.DeleteLink(l.ID); err != nil {
				return err
			}
		}
		log.Info("seed: updated existing profile", "slug", p.Slug, "id", p.ID)
	case errors.Is(err, store.ErrNotFound):
		id, err := st.CreateProfile(p)
		if err != nil {
			return err
		}
		p.ID = id
		log.Info("seed: created profile", "slug", p.Slug, "id", p.ID)
	default:
		return err
	}

	for i, ls := range spec.Links {
		if !services.IsLinkType(ls.Type) {
			return fmt.Errorf("seed link %d: unknown type %q", i+1, ls.Type)
		}
		dest, err := services.NormalizeLinkURL(ls.Type, ls.URL)
		if err != nil {
			return fmt.Errorf("seed link %d (%s): %w", i+1, ls.Type, err)
		}
		label := ls.Label
		if label == "" {
			label = services.DefaultLabel(ls.Type)
		}
		if _, err := st.CreateLink(&models.Link{
			ProfileID: p.ID,
			Type:      ls.Type,
			Label:     label,
			URL:       dest,
			Icon:      ls.Type,
			SortOrder: i + 1,
			Enabled:   true,
		}); err != nil {
			return err
		}
	}
	if spec.LogoFile != "" {
		path, err := installLogo(spec.LogoFile, uploadDir, p.ID)
		if err != nil {
			return err
		}
		p.LogoPath = path
		if err := st.UpdateProfile(p); err != nil {
			return err
		}
		log.Info("seed: installed logo", "slug", p.Slug, "path", path)
	}

	log.Info("seed: imported links", "slug", p.Slug, "count", len(spec.Links))
	return nil
}

// installLogo copies a bundled image into the upload directory under the
// profile-scoped name the upload handler also uses, and returns its public
// path.
func installLogo(name, uploadDir string, profileID int64) (string, error) {
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	body, err := dataFS.ReadFile("data/" + name)
	if err != nil {
		return "", fmt.Errorf("seed logo %q: %w", name, err)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", err
	}
	target := "profile-" + strconv.FormatInt(profileID, 10) + filepath.Ext(name)
	if err := os.WriteFile(filepath.Join(uploadDir, target), body, 0o644); err != nil {
		return "", err
	}
	return "/uploads/" + target, nil
}
