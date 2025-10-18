package embed

import (
	"embed"
	"io/fs"
)

//go:embed web/static
var staticFiles embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "web/static")
}
