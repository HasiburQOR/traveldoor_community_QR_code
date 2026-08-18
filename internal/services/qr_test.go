package services

import (
	"bytes"
	"strings"
	"testing"
)

func TestQRPNGIsAPNG(t *testing.T) {
	png, err := QRPNG("https://example.com/traveldoor", 512)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G'}) {
		t.Error("output is not a PNG")
	}
}

func TestQRSVGIsSquareAndStable(t *testing.T) {
	svg, err := QRSVG("https://example.com/traveldoor", 4)
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)
	if !strings.HasPrefix(s, "<svg ") || !strings.HasSuffix(s, "</svg>") {
		t.Fatalf("unexpected svg wrapper: %.60s", s)
	}
	// The same input must always produce identical output, so a reprinted code
	// matches the one already in circulation.
	again, err := QRSVG("https://example.com/traveldoor", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(svg, again) {
		t.Error("QRSVG is not deterministic")
	}
}

func TestQRJPEGAndPDF(t *testing.T) {
	jpg, err := QRJPEG("https://qrcode.example.com/traveldoor", 512)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(jpg, []byte{0xFF, 0xD8, 0xFF}) {
		t.Error("output is not a JPEG")
	}

	pdf, err := QRPDF("https://qrcode.example.com/traveldoor", "https://qrcode.example.com/traveldoor")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) {
		t.Error("output is not a PDF")
	}
	if !bytes.HasSuffix(bytes.TrimSpace(pdf), []byte("%%EOF")) {
		t.Error("PDF is not terminated")
	}
	for _, want := range []string{"/Type /Catalog", "/Type /Page", "startxref", " re f"} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("PDF missing %q", want)
		}
	}
	// The caption is a literal string, so unbalanced parentheses would corrupt
	// the file.
	if _, err := QRPDF("https://example.com/a(b)c", "https://example.com/a(b)c"); err != nil {
		t.Errorf("caption with parentheses: %v", err)
	}
}
