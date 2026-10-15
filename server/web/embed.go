// Package web embeds the built frontend into the binary. In dev, dist may be
// empty; staticFS then falls back to serving WebRoot from disk.
package web

import "embed"

// Assets holds the built frontend. The dist directory is populated by the
// frontend build before the backend is compiled for packaging.
//
//go:embed all:dist
var Assets embed.FS
