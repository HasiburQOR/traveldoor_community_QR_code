package services

import (
	"bytes"
	"fmt"
	"html"

	qrcode "github.com/skip2/go-qrcode"
)

// QRPNG renders the given text as a PNG QR code of roughly size pixels.
func QRPNG(text string, size int) ([]byte, error) {
	if size < 128 {
		size = 512
	}
	if size > 2048 {
		size = 2048
	}
	return qrcode.Encode(text, qrcode.Medium, size)
}

// QRSVG renders the given text as a scalable SVG QR code. Modules are emitted
// as one rect per dark module, which keeps the output dependency-free and
// prints at any size without loss.
func QRSVG(text string, quietZone int) ([]byte, error) {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	grid := q.Bitmap()
	// go-qrcode's bitmap already includes a quiet zone; trim it so the caller
	// controls the border.
	grid = trimQuietZone(grid)

	n := len(grid)
	if n == 0 {
		return nil, fmt.Errorf("empty qr bitmap")
	}
	if quietZone < 0 {
		quietZone = 4
	}
	dim := n + quietZone*2

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code for %s">`,
		dim, dim, html.EscapeString(text))
	fmt.Fprintf(&buf, `<rect width="%d" height="%d" fill="#ffffff"/>`, dim, dim)
	buf.WriteString(`<path fill="#000000" d="`)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if grid[y][x] {
				fmt.Fprintf(&buf, "M%d %dh1v1h-1z", x+quietZone, y+quietZone)
			}
		}
	}
	buf.WriteString(`"/></svg>`)
	return buf.Bytes(), nil
}

// trimQuietZone removes uniformly blank rows and columns from the edges.
func trimQuietZone(grid [][]bool) [][]bool {
	blankRow := func(row []bool) bool {
		for _, v := range row {
			if v {
				return false
			}
		}
		return true
	}
	top := 0
	for top < len(grid) && blankRow(grid[top]) {
		top++
	}
	bottom := len(grid)
	for bottom > top && blankRow(grid[bottom-1]) {
		bottom--
	}
	if top >= bottom {
		return grid
	}
	rows := grid[top:bottom]

	width := len(rows[0])
	blankCol := func(x int) bool {
		for _, row := range rows {
			if x < len(row) && row[x] {
				return false
			}
		}
		return true
	}
	left := 0
	for left < width && blankCol(left) {
		left++
	}
	right := width
	for right > left && blankCol(right-1) {
		right--
	}

	out := make([][]bool, len(rows))
	for i, row := range rows {
		out[i] = row[left:right]
	}
	return out
}
