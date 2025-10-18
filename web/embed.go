package web

import (
	"embed"
	"io/fs"
)

//go:embed static/js static/styles/output.css
var staticFiles embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "static")
}
