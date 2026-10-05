package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed js/*.js
var jsFS embed.FS

//go:embed css/*.css
var cssFS embed.FS

//go:embed img/*
var imgFS embed.FS

//go:embed fonts/*.woff2
var fontsFS embed.FS

func JSFS() fs.FS {
	sub, err := fs.Sub(jsFS, "js")
	if err != nil {
		panic(err)
	}
	return sub
}

func JSHandler() http.Handler {
	return http.StripPrefix("/static/btk/js/", FSHandler(JSFS()))
}

func CSSFS() fs.FS {
	sub, err := fs.Sub(cssFS, "css")
	if err != nil {
		panic(err)
	}
	return sub
}

func CSSHandler() http.Handler {
	return http.StripPrefix("/static/btk/css/", FSHandler(CSSFS()))
}

func ImgFS() fs.FS {
	sub, err := fs.Sub(imgFS, "img")
	if err != nil {
		panic(err)
	}
	return sub
}

func ImgHandler() http.Handler {
	return http.StripPrefix("/static/btk/img/", FSHandler(ImgFS()))
}

func FontsFS() fs.FS {
	sub, err := fs.Sub(fontsFS, "fonts")
	if err != nil {
		panic(err)
	}
	return sub
}

func FontHandler() http.Handler {
	return http.StripPrefix("/static/btk/fonts/", FSHandler(FontsFS()))
}
