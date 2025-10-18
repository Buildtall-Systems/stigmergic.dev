package embed

import (
	"embed"
	"io/fs"
)

//go:embed ../../web/static/js ../../web/static/styles/output.css
var staticFiles embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "web/static")
}
