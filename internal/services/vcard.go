package services

import (
	"fmt"
	"strings"

	"traveldoor/qrprofile/internal/models"
)

// VCard renders a vCard 3.0 contact card for a profile. publicURL is the
// canonical profile URL that the printed QR code encodes.
func VCard(p *models.Profile, publicURL string, links []*models.Link) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
	fmt.Fprintf(&b, "N:;%s;;;\r\n", vEscape(p.Name))
	fmt.Fprintf(&b, "FN:%s\r\n", vEscape(p.Name))
	fmt.Fprintf(&b, "ORG:%s\r\n", vEscape(p.Name))
	if p.Subtitle != "" {
		fmt.Fprintf(&b, "TITLE:%s\r\n", vEscape(p.Subtitle))
	}
	if p.Phone != "" {
		fmt.Fprintf(&b, "TEL;TYPE=WORK,VOICE:%s\r\n", vEscape(p.Phone))
	}
	if p.Email != "" {
		fmt.Fprintf(&b, "EMAIL;TYPE=INTERNET,WORK:%s\r\n", vEscape(p.Email))
	}
	if p.Address != "" {
		fmt.Fprintf(&b, "ADR;TYPE=WORK:;;%s;;;;\r\n", vEscape(p.Address))
	}
	if p.Website != "" {
		fmt.Fprintf(&b, "URL:%s\r\n", vEscape(p.Website))
	}
	if publicURL != "" {
		fmt.Fprintf(&b, "URL;TYPE=PROFILE:%s\r\n", vEscape(publicURL))
	}
	for _, l := range links {
		if !l.Enabled {
			continue
		}
		switch l.Type {
		case "facebook", "instagram", "tiktok", "youtube", "linkedin", "x":
			fmt.Fprintf(&b, "X-SOCIALPROFILE;TYPE=%s:%s\r\n", strings.ToUpper(l.Type), vEscape(l.URL))
		}
	}
	if p.Description != "" {
		fmt.Fprintf(&b, "NOTE:%s\r\n", vEscape(p.Description))
	}
	b.WriteString("END:VCARD\r\n")
	return []byte(b.String())
}

// vEscape escapes the vCard delimiters and strips line breaks.
func vEscape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "",
	)
	return r.Replace(s)
}
