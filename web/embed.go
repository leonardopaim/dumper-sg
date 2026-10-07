package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// FS returns the built frontend assets, relative to dist.
func FS() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
