// Package web embeds the versioned interface translation resources.
package web

import "embed"

//go:embed locales/*.json
var Locales embed.FS
