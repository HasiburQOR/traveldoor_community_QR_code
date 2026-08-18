package seed

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"traveldoor/qrprofile/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestImportDefaultIsIdempotent(t *testing.T) {
	st := newStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := ImportDefault(st, t.TempDir(), log); err != nil {
		t.Fatalf("first import: %v", err)
	}
	p, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatalf("profile not created: %v", err)
	}
	if !p.Published {
		t.Error("seeded profile should be published")
	}
	if p.Phone != "+995591177208" || p.Email != "office@traveldoor.ge" {
		t.Errorf("contact details = %q / %q", p.Phone, p.Email)
	}

	first, err := st.ListLinks(p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("no links seeded")
	}

	// Re-importing must not duplicate links or change the id (and therefore
	// the slug and the printed QR).
	if err := ImportDefault(st, t.TempDir(), log); err != nil {
		t.Fatalf("second import: %v", err)
	}
	again, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != p.ID {
		t.Errorf("profile id changed: %d -> %d", p.ID, again.ID)
	}
	second, err := st.ListLinks(again.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("link count = %d after re-import, want %d", len(second), len(first))
	}

	// Ordering and normalisation come from the seed file.
	want := []struct{ typ, dest string }{
		{"facebook", "https://www.facebook.com/Traveldoorgeorgia"},
		{"instagram", "https://www.instagram.com/traveldoorgeorgia/"},
		{"tiktok", "https://www.tiktok.com/@travel_door"},
		{"youtube", "https://www.youtube.com/@traveldoorge"},
		{"linkedin", "https://www.linkedin.com/company/travel-door-georgia"},
		{"x", "https://x.com/TraveldoorGe"},
		{"website", "https://traveldoor.ge"},
		{"email", "mailto:office@traveldoor.ge"},
		{"phone", "tel:+995591177208"},
		{"whatsapp", "https://wa.me/995591177208"},
	}
	if len(second) != len(want) {
		t.Fatalf("seeded %d links, want %d", len(second), len(want))
	}
	for i, w := range want {
		if second[i].Type != w.typ || second[i].URL != w.dest {
			t.Errorf("link %d = %s %s, want %s %s", i, second[i].Type, second[i].URL, w.typ, w.dest)
		}
	}
}

func TestSeedInstallsLogoAndKeepsAdminUploads(t *testing.T) {
	st := newStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	uploads := t.TempDir()

	if err := ImportDefault(st, uploads, log); err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatal(err)
	}
	if p.LogoPath == "" {
		t.Fatal("seed did not install the bundled logo")
	}
	if _, err := os.Stat(filepath.Join(uploads, filepath.Base(p.LogoPath))); err != nil {
		t.Fatalf("logo file not written: %v", err)
	}

	// A logo uploaded through the admin must survive a later re-seed that
	// carries no logo of its own.
	p.LogoPath = "/uploads/admin-choice.png"
	if err := st.UpdateProfile(p); err != nil {
		t.Fatal(err)
	}
	body, err := dataFS.ReadFile("data/traveldoor.json")
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(body), `"logo_file": "traveldoor-logo.webp",`, "", 1)
	if strings.Contains(stripped, "logo_file") {
		t.Fatal("test fixture did not strip logo_file")
	}
	if err := importJSON(st, []byte(stripped), uploads, log); err != nil {
		t.Fatal(err)
	}
	after, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatal(err)
	}
	if after.LogoPath != "/uploads/admin-choice.png" {
		t.Errorf("LogoPath = %q, want the admin upload to be preserved", after.LogoPath)
	}
}

func TestImportDefaultIfMissingLeavesAdminEditsAlone(t *testing.T) {
	st := newStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	uploads := t.TempDir()

	if err := ImportDefaultIfMissing(st, uploads, log); err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the admin curating the link set.
	links, err := st.ListLinks(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLink(links[0].ID); err != nil {
		t.Fatal(err)
	}
	p.Name = "Renamed In Admin"
	if err := st.UpdateProfile(p); err != nil {
		t.Fatal(err)
	}

	// A restart must not undo any of that.
	if err := ImportDefaultIfMissing(st, uploads, log); err != nil {
		t.Fatal(err)
	}
	after, err := st.ProfileBySlug("traveldoor")
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Renamed In Admin" {
		t.Errorf("name = %q, want the admin edit preserved", after.Name)
	}
	remaining, err := st.ListLinks(after.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != len(links)-1 {
		t.Errorf("links = %d, want %d (the deleted link must stay deleted)", len(remaining), len(links)-1)
	}
}
