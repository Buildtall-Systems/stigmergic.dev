package static

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// Cache-Control policies, matched to how often an asset changes under a fixed
// name. See cacheControlFor.
const (
	cacheRevalidate = "no-cache"                            // JS/CSS: change silently across deploys
	cacheImages     = "public, max-age=604800"              // images: rare, deliberate changes
	cacheImmutable  = "public, max-age=31536000, immutable" // fonts: content-pinned by name
)

// FSHandler serves files from fsys with content-derived validators and a
// per-extension Cache-Control policy. It deliberately serves with a zero
// modtime so http.ServeContent emits no Last-Modified header: on the Nix
// store every file's mtime is normalized to a constant, which makes
// Last-Modified a useless validator that returns 304 even after a deploy
// changed the content. The strong validator computed here is content-based,
// so revalidation returns 304 only when the bytes genuinely match.
//
// The caller is responsible for stripping the URL prefix (as with
// http.FileServer), so r.URL.Path is the path within fsys.
func FSHandler(fsys fs.FS) http.Handler {
	return &cacheFS{fsys: fsys}
}

// DirHandler serves an on-disk directory with the same policy as FSHandler.
// It replaces http.FileServer(http.Dir(dir)) at application call sites.
func DirHandler(dir string) http.Handler {
	return FSHandler(os.DirFS(dir))
}

type cacheFS struct {
	fsys  fs.FS
	etags sync.Map // path -> quoted validator string
}

func (c *cacheFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if name == "." || !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}

	f, err := c.fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Debug("closing static file", "name", name, "error", closeErr)
		}
	}()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		rs = bytes.NewReader(data)
	}

	etag, err := c.validatorFor(name, rs)
	if err != nil {
		http.Error(w, "validator error", http.StatusInternalServerError)
		return
	}
	if etag != "" {
		w.Header().Set("Etag", etag)
	}
	w.Header().Set("Cache-Control", cacheControlFor(name))

	http.ServeContent(w, r, info.Name(), time.Time{}, rs)
}

// validatorFor returns the content-derived validator for name, computing it
// from rs on first use and memoizing it. Assets are immutable within a
// process, so the memoized value stays correct; a redeploy starts a fresh
// process and recomputes. rs is left seeked to the start for ServeContent.
func (c *cacheFS) validatorFor(name string, rs io.ReadSeeker) (string, error) {
	if v, loaded := c.etags.Load(name); loaded {
		if etag, ok := v.(string); ok {
			return etag, nil
		}
	}

	h := sha256.New()
	if _, err := io.Copy(h, rs); err != nil {
		return "", err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	etag := `"` + hex.EncodeToString(h.Sum(nil)) + `"`
	c.etags.Store(name, etag)
	return etag, nil
}

// cacheControlFor matches caching strength to how often an asset changes
// under a fixed name. JS/CSS change silently across deploys, so they must
// revalidate every load; fonts are content-pinned by name and effectively
// never change; images change rarely and deliberately.
func cacheControlFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".woff2", ".woff", ".ttf", ".otf", ".eot":
		return cacheImmutable
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".avif":
		return cacheImages
	default: // .js, .css, and anything else
		return cacheRevalidate
	}
}
