package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFS embed.FS

// FS returns the embedded frontend build rooted at the dist/ directory.
func FS() fs.FS {
	sub, err := fs.Sub(embeddedFS, "dist")
	if err != nil {
		panic("static: embed.Sub failed: " + err.Error())
	}
	return sub
}
