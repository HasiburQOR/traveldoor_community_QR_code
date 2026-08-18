// Package services holds the small pieces of domain logic that are not pure
// storage: validation/normalisation, QR rendering and vCard generation.
package services

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

// LinkType describes one supported destination kind.
type LinkType struct {
	Key         string
	Label       string
	Placeholder string
}

// LinkTypes is the allowlist of destination kinds. Anything else is rejected.
var LinkTypes = []LinkType{
	{"website", "Website", "https://example.com"},
	{"facebook", "Facebook", "https://www.facebook.com/yourpage"},
	{"instagram", "Instagram", "https://www.instagram.com/yourhandle/"},
	{"tiktok", "TikTok", "https://www.tiktok.com/@yourhandle"},
	{"youtube", "YouTube", "https://www.youtube.com/@yourchannel"},
	{"linkedin", "LinkedIn", "https://www.linkedin.com/company/yourcompany"},
	{"x", "X (Twitter)", "https://x.com/yourhandle"},
	{"whatsapp", "WhatsApp", "+995 5xx xx xx xx"},
	{"phone", "Phone", "+995 5xx xx xx xx"},
	{"email", "Email", "office@example.com"},
	{"map", "Map / Address", "https://maps.google.com/?q=..."},
	{"generic", "Other link", "https://example.com/page"},
}

func IsLinkType(key string) bool {
	for _, t := range LinkTypes {
		if t.Key == key {
			return true
		}
	}
	return false
}

var (
	slugRe  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	phoneRe = regexp.MustCompile(`^\+?[0-9]{6,15}$`)

	ErrEmptyValue = errors.New("value is required")
)

// NormalizeSlug lowercases and strips a slug down to url-safe characters.
func NormalizeSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ValidateSlug rejects slugs that are empty, malformed or reserved.
func ValidateSlug(slug string) error {
	if slug == "" {
		return errors.New("slug is required")
	}
	if len(slug) > 64 {
		return errors.New("slug must be 64 characters or fewer")
	}
	if !slugRe.MatchString(slug) {
		return errors.New("slug may contain only lowercase letters, numbers and single hyphens")
	}
	if ReservedSlugs[slug] {
		return fmt.Errorf("%q is reserved", slug)
	}
	return nil
}

// ReservedSlugs are first path segments the application itself owns.
var ReservedSlugs = map[string]bool{
	"admin": true, "static": true, "uploads": true, "healthz": true,
	"favicon.ico": true, "robots.txt": true, "api": true, "go": true,
}

// NormalizePhone keeps digits and a single leading plus.
func NormalizePhone(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmptyValue
	}
	plus := strings.HasPrefix(s, "+")
	digits := regexp.MustCompile(`\D`).ReplaceAllString(s, "")
	if plus {
		digits = "+" + digits
	}
	if !phoneRe.MatchString(digits) {
		return "", errors.New("phone number looks invalid")
	}
	return digits, nil
}

func ValidateEmail(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmptyValue
	}
	if _, err := mail.ParseAddress(s); err != nil {
		return "", errors.New("email address looks invalid")
	}
	return s, nil
}

// safeSchemes is the allowlist for rendered destinations. javascript:, data:,
// vbscript:, file: and everything else are rejected.
var safeSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tel": true}

// ValidateURL parses raw, defaults a bare host to https and rejects any scheme
// outside the allowlist.
func ValidateURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmptyValue
	}
	if strings.ContainsAny(s, "\r\n\t") {
		return "", errors.New("url contains control characters")
	}
	if !strings.Contains(s, ":") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("url could not be parsed")
	}
	scheme := strings.ToLower(u.Scheme)
	if !safeSchemes[scheme] {
		return "", fmt.Errorf("%q links are not allowed", scheme)
	}
	if (scheme == "http" || scheme == "https") && u.Host == "" {
		return "", errors.New("url is missing a host")
	}
	u.Scheme = scheme
	return u.String(), nil
}

// NormalizeLinkURL converts the admin-entered value for a link type into the
// canonical destination that will be rendered on the public page.
func NormalizeLinkURL(linkType, raw string) (string, error) {
	switch linkType {
	case "phone":
		p, err := NormalizePhone(raw)
		if err != nil {
			return "", err
		}
		return "tel:" + p, nil
	case "whatsapp":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "http") {
			return ValidateURL(raw)
		}
		p, err := NormalizePhone(raw)
		if err != nil {
			return "", err
		}
		// wa.me expects digits only; the https form keeps a web fallback when
		// the app is not installed.
		return "https://wa.me/" + strings.TrimPrefix(p, "+"), nil
	case "email":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "mailto:") {
			return ValidateURL(raw)
		}
		e, err := ValidateEmail(raw)
		if err != nil {
			return "", err
		}
		return "mailto:" + e, nil
	default:
		if !IsLinkType(linkType) {
			return "", errors.New("unknown link type")
		}
		return ValidateURL(raw)
	}
}

// DefaultLabel returns the human label for a link type, used when the admin
// leaves the label field blank.
func DefaultLabel(key string) string {
	for _, t := range LinkTypes {
		if t.Key == key {
			return t.Label
		}
	}
	return "Link"
}
