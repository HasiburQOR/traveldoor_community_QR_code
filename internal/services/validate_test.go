package services

import "testing"

func TestValidateURLRejectsUnsafeSchemes(t *testing.T) {
	unsafe := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"",
		"https://exa\nmple.com",
	}
	for _, raw := range unsafe {
		if got, err := ValidateURL(raw); err == nil {
			t.Errorf("ValidateURL(%q) = %q, want error", raw, got)
		}
	}
}

func TestValidateURLAcceptsAndNormalises(t *testing.T) {
	cases := map[string]string{
		"https://x.com/TraveldoorGe": "https://x.com/TraveldoorGe",
		"traveldoor.ge":              "https://traveldoor.ge",
		"HTTPS://Traveldoor.ge/page": "https://Traveldoor.ge/page",
	}
	for in, want := range cases {
		got, err := ValidateURL(in)
		if err != nil {
			t.Fatalf("ValidateURL(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("ValidateURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLinkURL(t *testing.T) {
	cases := []struct{ typ, in, want string }{
		{"phone", "+995 591 17 72 08", "tel:+995591177208"},
		{"email", "office@traveldoor.ge", "mailto:office@traveldoor.ge"},
		{"whatsapp", "+995 591 17 72 08", "https://wa.me/995591177208"},
		{"facebook", "https://www.facebook.com/Traveldoorgeorgia", "https://www.facebook.com/Traveldoorgeorgia"},
	}
	for _, c := range cases {
		got, err := NormalizeLinkURL(c.typ, c.in)
		if err != nil {
			t.Fatalf("NormalizeLinkURL(%q, %q) error: %v", c.typ, c.in, err)
		}
		if got != c.want {
			t.Errorf("NormalizeLinkURL(%q, %q) = %q, want %q", c.typ, c.in, got, c.want)
		}
	}
	if _, err := NormalizeLinkURL("unknown-type", "https://example.com"); err == nil {
		t.Error("expected unknown link type to be rejected")
	}
	if _, err := NormalizeLinkURL("phone", "not-a-number"); err == nil {
		t.Error("expected invalid phone number to be rejected")
	}
}

func TestSlugRules(t *testing.T) {
	if got := NormalizeSlug("  Travel Door  Georgia! "); got != "travel-door-georgia" {
		t.Errorf("NormalizeSlug = %q", got)
	}
	if err := ValidateSlug("travel-door"); err != nil {
		t.Errorf("valid slug rejected: %v", err)
	}
	for _, bad := range []string{"", "Admin", "admin", "static", "has space", "-lead", "trail-", "double--hyphen"} {
		if err := ValidateSlug(bad); err == nil {
			t.Errorf("ValidateSlug(%q) accepted, want error", bad)
		}
	}
}
