package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"traveldoor/qrprofile/internal/auth"
)

// pngBytes returns a tiny valid PNG.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 11, G: 107, B: 91, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// postProfileWithFile submits the profile edit form as multipart with one file.
func (a *testApp) postProfileWithFile(path string, fields map[string]string, fileName string, content []byte) *http.Response {
	a.t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField(auth.CSRFField, a.csrfToken()); err != nil {
		a.t.Fatal(err)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			a.t.Fatal(err)
		}
	}
	if fileName != "" {
		part, err := mw.CreateFormFile("logo", fileName)
		if err != nil {
			a.t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			a.t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		a.t.Fatal(err)
	}

	resp, err := a.client.Post(a.server.URL+path, mw.FormDataContentType(), &body)
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func TestLogoUploadIsStoredAndServed(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)

	fields := map[string]string{"name": p.Name, "slug": p.Slug}
	resp := app.postProfileWithFile("/admin/profiles/"+strconv.FormatInt(p.ID, 10),
		fields, "logo.png", pngBytes(t))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	saved, err := app.store.ProfileByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := "/uploads/profile-" + strconv.FormatInt(p.ID, 10) + ".png"
	if saved.LogoPath != want {
		t.Fatalf("LogoPath = %q, want %q", saved.LogoPath, want)
	}

	img := app.get(want)
	img.Body.Close()
	if img.StatusCode != http.StatusOK {
		t.Errorf("serving the logo returned %d", img.StatusCode)
	}

	// The public page must reference it.
	if body := app.body(app.get("/traveldoor")); !strings.Contains(body, want) {
		t.Error("public page does not use the uploaded logo")
	}
}

func TestNonImageLogoIsRejected(t *testing.T) {
	app := newTestApp(t)
	app.signIn()
	p := app.seedProfile(true)

	fields := map[string]string{"name": p.Name, "slug": p.Slug}
	// An SVG is a script-capable document, so it must not be accepted.
	resp := app.postProfileWithFile("/admin/profiles/"+strconv.FormatInt(p.ID, 10),
		fields, "logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	body := app.body(resp)
	if !strings.Contains(body, "Logo:") {
		t.Error("expected a logo validation error to be shown")
	}

	saved, err := app.store.ProfileByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LogoPath != "" {
		t.Errorf("LogoPath = %q, want empty", saved.LogoPath)
	}
}

func TestUploadPathTraversalIsRejected(t *testing.T) {
	app := newTestApp(t)

	for _, path := range []string{"/uploads/..%2f..%2fapp.db", "/uploads/.hidden"} {
		resp := app.get(path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}
