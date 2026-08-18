package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"traveldoor/qrprofile/internal/auth"
	"traveldoor/qrprofile/internal/config"
	"traveldoor/qrprofile/internal/models"
	"traveldoor/qrprofile/internal/store"
)

type testApp struct {
	t      *testing.T
	store  *store.Store
	server *httptest.Server
	client *http.Client
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := &config.Config{
		AppEnv:        "test",
		BaseURL:       "https://qr.example.com",
		SessionSecret: "test-secret",
		UploadDir:     filepath.Join(t.TempDir(), "uploads"),
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv, err := New(cfg, st, log)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &testApp{t: t, store: st, server: ts, client: client}
}

func (a *testApp) get(path string) *http.Response {
	a.t.Helper()
	resp, err := a.client.Get(a.server.URL + path)
	if err != nil {
		a.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (a *testApp) body(resp *http.Response) string {
	a.t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	return string(b)
}

// post submits a form with the CSRF token held in the cookie jar.
func (a *testApp) post(path string, form url.Values) *http.Response {
	a.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set(auth.CSRFField, a.csrfToken())

	resp, err := a.client.PostForm(a.server.URL+path, form)
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// csrfToken primes the cookie jar by loading the login page if needed.
func (a *testApp) csrfToken() string {
	a.t.Helper()
	u, _ := url.Parse(a.server.URL)
	find := func() string {
		for _, c := range a.client.Jar.Cookies(u) {
			if c.Name == auth.CSRFCookie {
				return c.Value
			}
		}
		return ""
	}
	if v := find(); v != "" {
		return v
	}
	a.get("/admin/login").Body.Close()
	if v := find(); v != "" {
		return v
	}
	a.t.Fatal("no csrf cookie issued")
	return ""
}

func (a *testApp) signIn() {
	a.t.Helper()
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := a.store.CreateUser("admin@example.com", hash); err != nil {
		a.t.Fatal(err)
	}
	resp := a.post("/admin/login", url.Values{
		"email":    {"admin@example.com"},
		"password": {"correct-horse-battery"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		a.t.Fatalf("login status = %d, want 303", resp.StatusCode)
	}
}

func (a *testApp) seedProfile(published bool) *models.Profile {
	a.t.Helper()
	p := &models.Profile{
		Slug:      "traveldoor",
		Name:      "Travel Door Georgia",
		Subtitle:  "Join Our Travel Door Georgia Community",
		ThemeJSON: "{}",
		Published: published,
	}
	id, err := a.store.CreateProfile(p)
	if err != nil {
		a.t.Fatal(err)
	}
	p.ID = id
	return p
}

func TestPublishedSlugIsServedAndUnpublishedIsNot(t *testing.T) {
	app := newTestApp(t)
	p := app.seedProfile(false)

	resp := app.get("/traveldoor")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unpublished slug status = %d, want 404", resp.StatusCode)
	}

	if err := app.store.SetProfilePublished(p.ID, true); err != nil {
		t.Fatal(err)
	}
	resp = app.get("/traveldoor")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("published slug status = %d, want 200", resp.StatusCode)
	}
	if body := app.body(resp); !strings.Contains(body, "Travel Door Georgia") {
		t.Error("public page does not contain the profile name")
	}

	resp = app.get("/does-not-exist")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown slug status = %d, want 404", resp.StatusCode)
	}
}

func TestDisabledLinkDisappearsFromPublicPage(t *testing.T) {
	app := newTestApp(t)
	p := app.seedProfile(true)

	l := &models.Link{
		ProfileID: p.ID,
		Type:      "instagram",
		Label:     "Instagram",
		URL:       "https://www.instagram.com/traveldoorgeorgia/",
		Enabled:   true,
	}
	id, err := app.store.CreateLink(l)
	if err != nil {
		t.Fatal(err)
	}
	l.ID = id

	if body := app.body(app.get("/traveldoor")); !strings.Contains(body, "Instagram") {
		t.Fatal("enabled link missing from public page")
	}

	l.Enabled = false
	if err := app.store.UpdateLink(l); err != nil {
		t.Fatal(err)
	}
	if body := app.body(app.get("/traveldoor")); strings.Contains(body, "Instagram") {
		t.Error("disabled link is still rendered")
	}

	// A disabled link must also stop redirecting.
	resp := app.get("/go/" + strconv.FormatInt(l.ID, 10))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("disabled link redirect status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkRedirectRecordsClickAndSendsToDestination(t *testing.T) {
	app := newTestApp(t)
	p := app.seedProfile(true)
	id, err := app.store.CreateLink(&models.Link{
		ProfileID: p.ID, Type: "x", Label: "X",
		URL: "https://x.com/TraveldoorGe", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := app.get("/go/" + strconv.FormatInt(id, 10))
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("redirect status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://x.com/TraveldoorGe" {
		t.Errorf("Location = %q", got)
	}

	stats, err := app.store.ProfileStats(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Clicks != 1 || stats.PerLink[id] != 1 {
		t.Errorf("clicks = %d, per-link = %d, want 1 and 1", stats.Clicks, stats.PerLink[id])
	}
}

func TestAdminRoutesRequireAuthentication(t *testing.T) {
	app := newTestApp(t)
	resp := app.get("/admin")
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect to login", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin/login" {
		t.Errorf("Location = %q, want /admin/login", loc)
	}
}

func TestMutationWithoutCSRFTokenIsRejected(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)

	resp, err := app.client.PostForm(
		app.server.URL+"/admin/profiles/"+strconv.FormatInt(p.ID, 10)+"/links",
		url.Values{"type": {"website"}, "url": {"https://traveldoor.ge"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminRejectsUnsafeLinkScheme(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)

	resp := app.post("/admin/profiles/"+strconv.FormatInt(p.ID, 10)+"/links",
		url.Values{"type": {"website"}, "label": {"Bad"}, "url": {"javascript:alert(1)"}})
	resp.Body.Close()

	links, err := app.store.ListLinks(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("stored %d links, want 0", len(links))
	}
}

func TestQRStaysStableWhenContentChanges(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)
	path := "/admin/profiles/" + strconv.FormatInt(p.ID, 10) + "/qr.svg"

	before := app.body(app.get(path))

	// Change the content, but not the slug.
	if _, err := app.store.CreateLink(&models.Link{
		ProfileID: p.ID, Type: "tiktok", Label: "TikTok",
		URL: "https://www.tiktok.com/@travel_door", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	p.Name = "Travel Door Georgia LLC"
	if err := app.store.UpdateProfile(p); err != nil {
		t.Fatal(err)
	}

	after := app.body(app.get(path))
	if before != after {
		t.Error("QR changed after a content-only edit")
	}
	if !strings.Contains(before, "https://qr.example.com/traveldoor") {
		t.Error("QR does not encode the canonical public URL")
	}
}

func TestLinkOrderingIsPersisted(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)

	var ids []int64
	for _, spec := range []struct{ typ, dest string }{
		{"facebook", "https://www.facebook.com/Traveldoorgeorgia"},
		{"instagram", "https://www.instagram.com/traveldoorgeorgia/"},
		{"youtube", "https://www.youtube.com/@traveldoorge"},
	} {
		id, err := app.store.CreateLink(&models.Link{
			ProfileID: p.ID, Type: spec.typ, Label: spec.typ,
			URL: spec.dest, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	// Move the last link to the front.
	order := strconv.FormatInt(ids[2], 10) + "," +
		strconv.FormatInt(ids[0], 10) + "," +
		strconv.FormatInt(ids[1], 10)
	app.post("/admin/profiles/"+strconv.FormatInt(p.ID, 10)+"/links/reorder",
		url.Values{"order": {order}}).Body.Close()

	links, err := app.store.ListLinks(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{ids[2], ids[0], ids[1]}
	for i := range want {
		if links[i].ID != want[i] {
			t.Fatalf("order = %d,%d,%d want %v", links[0].ID, links[1].ID, links[2].ID, want)
		}
	}

	// The single-step form must move a link down by one position.
	app.post("/admin/profiles/"+strconv.FormatInt(p.ID, 10)+"/links/reorder",
		url.Values{"id": {strconv.FormatInt(ids[2], 10)}, "direction": {"down"}}).Body.Close()

	links, err = app.store.ListLinks(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if links[0].ID != ids[0] || links[1].ID != ids[2] {
		t.Errorf("after move down: %d,%d,%d", links[0].ID, links[1].ID, links[2].ID)
	}
}

func TestVCardContainsContactDetails(t *testing.T) {
	app := newTestApp(t)
	p := app.seedProfile(true)
	p.Phone = "+995591177208"
	p.Email = "office@traveldoor.ge"
	p.Website = "https://traveldoor.ge"
	if err := app.store.UpdateProfile(p); err != nil {
		t.Fatal(err)
	}

	resp := app.get("/traveldoor/contact.vcf")
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/vcard") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := app.body(resp)
	for _, want := range []string{
		"BEGIN:VCARD", "FN:Travel Door Georgia", "TEL", "+995591177208",
		"office@traveldoor.ge", "https://qr.example.com/traveldoor", "END:VCARD",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("vcard missing %q", want)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	app := newTestApp(t)
	resp := app.get("/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if body := app.body(resp); !strings.Contains(body, "ok") {
		t.Errorf("body = %q", body)
	}
}
