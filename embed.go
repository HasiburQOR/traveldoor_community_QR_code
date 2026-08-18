// Package qrprofile exposes the application's embedded templates and static
// assets so the compiled binary is self-contained.
package qrprofile

import "embed"

//go:embed templates
var TemplateFS embed.FS

//go:embed static
var StaticFS embed.FS
