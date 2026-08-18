package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// QRJPEG renders the QR as a JPEG. JPEG has no transparency and is lossy, so
// it exists only for tools that refuse anything else; PNG or SVG is better.
func QRJPEG(text string, size int) ([]byte, error) {
	raw, err := QRPNG(text, size)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	// Flatten onto white so no alpha is silently turned black.
	flat := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			flat.Set(x, y, img.At(x, y))
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: 95}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// QRPDF renders a print-ready A4 page: the QR drawn as vector rectangles (so
// it stays sharp at any print size) with the encoded URL captioned underneath.
func QRPDF(text, caption string) ([]byte, error) {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	grid := trimQuietZone(q.Bitmap())
	n := len(grid)
	if n == 0 {
		return nil, fmt.Errorf("empty qr bitmap")
	}

	const (
		pageW  = 595.28 // A4 at 72dpi
		pageH  = 841.89
		qrSide = 320.0
	)
	module := qrSide / float64(n)
	originX := (pageW - qrSide) / 2
	originY := (pageH-qrSide)/2 + 40

	var content bytes.Buffer
	content.WriteString("0 0 0 rg\n")
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if !grid[y][x] {
				continue
			}
			// PDF's origin is bottom-left, the bitmap's is top-left.
			px := originX + float64(x)*module
			py := originY + qrSide - float64(y+1)*module
			fmt.Fprintf(&content, "%.3f %.3f %.3f %.3f re f\n", px, py, module, module)
		}
	}
	if caption != "" {
		fmt.Fprintf(&content, "BT /F1 11 Tf %.2f %.2f Td (%s) Tj ET\n",
			originX, originY-28, pdfEscape(caption))
	}

	return buildPDF(content.Bytes(), pageW, pageH), nil
}

// pdfEscape escapes the characters that terminate a PDF literal string.
func pdfEscape(s string) string {
	r := strings.NewReplacer(`\`, `\`, `(`, `\(`, `)`, `\)`, "\r", "", "\n", " ")
	return r.Replace(s)
}

// buildPDF assembles a single-page PDF around a content stream, tracking byte
// offsets for the cross-reference table.
func buildPDF(content []byte, pageW, pageH float64) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] "+
			"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", pageW, pageH),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)

	for i, body := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}

	offsets = append(offsets, buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<< /Length %d >>\nstream\n", len(objects)+1, len(content))
	buf.Write(content)
	buf.WriteString("endstream\nendobj\n")

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets)+1, xref)
	return buf.Bytes()
}
