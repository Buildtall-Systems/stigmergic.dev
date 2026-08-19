package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/buildtall-systems/buildtall/btk/okf"
)

// conceptExt is the extension a concept file carries inside the mount: the
// ConceptID plus this suffix is the file's path, and trimming it recovers
// the ConceptID.
const conceptExt = ".md"

// opOpen names the operation every open-path error reports.
const opOpen = "open"

// FS is the synthetic read-only filesystem over one vault's bundle:
// directories from the bundle tree, each concept a markdown file whose bytes
// are okf.WriteConcept output so frontmatter titles survive, each attachment
// a file whose bytes arrive from the vault's stores on first use, verified
// against the stated hash and materialized behind a bytes reader so
// http.ServeFileFS can seek. A failed or unverified fetch is a not-found,
// never a panic.
type FS struct {
	entries map[string]*entry
	client  *http.Client
	// ctx bounds blob fetches for the life of the serve; fs.FS carries no
	// per-call context of its own.
	ctx  context.Context
	name string
	// servers trails for field alignment: its len and cap words keep the
	// GC's pointer-scan region off the end of the struct.
	servers []string
}

// entry is one path in the filesystem. A directory carries its sorted child
// names; a concept carries its bytes; an attachment carries its stated hash
// and materializes its blob once, on first stat or open.
type entry struct {
	// mod leads and blob trails the pointer-bearing fields for field
	// alignment: the order keeps the GC's pointer-scan region tight.
	mod      time.Time
	blobErr  error
	children []string
	data     []byte
	sha      string
	blob     []byte
	once     sync.Once
	dir      bool
}

// NewFS builds the filesystem from an exported bundle. name is what the root
// directory's stat reports, the vault's own name; modTimes stamps concept
// files from their document events; servers are the vault's stores in
// preference order; a nil client means http.DefaultClient.
func NewFS(ctx context.Context, b *okf.Bundle, name string, modTimes map[string]time.Time, servers []string, client *http.Client) (*FS, error) {
	if client == nil {
		client = http.DefaultClient
	}
	f := &FS{
		entries: map[string]*entry{},
		client:  client,
		ctx:     ctx,
		servers: servers,
		name:    name,
	}
	if err := f.index(b.Root, modTimes); err != nil {
		return nil, err
	}
	return f, nil
}

// index files one bundle directory and everything beneath it.
func (f *FS) index(d *okf.Directory, modTimes map[string]time.Time) error {
	children := d.Children()
	names := make([]string, 0, len(d.Concepts)+len(d.Attachments)+len(children))

	for _, c := range d.Concepts {
		data, err := okf.WriteConcept(c)
		if err != nil {
			return fmt.Errorf("serializing concept %q: %w", c.ConceptID, err)
		}
		fp := c.ConceptID + conceptExt
		f.entries[fp] = &entry{data: data, mod: modTimes[fp]}
		names = append(names, path.Base(fp))
	}
	for _, a := range d.Attachments {
		f.entries[joinDirPath(d.Path, a.Name)] = &entry{sha: a.SHA256}
		names = append(names, a.Name)
	}
	for _, child := range children {
		if err := f.index(child, modTimes); err != nil {
			return err
		}
		names = append(names, path.Base(child.Path))
	}

	slices.Sort(names)
	key := d.Path
	if key == "" {
		key = "."
	}
	f.entries[key] = &entry{dir: true, children: names}
	return nil
}

// joinDirPath joins a bundle-relative directory path, empty at the root, to
// a name inside it.
func joinDirPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// Open answers fs.FS. Directories open as fs.ReadDirFile; a concept opens
// over its serialized bytes; an attachment materializes its blob first, and
// a fetch that fails or hashes wrong reports fs.ErrNotExist.
func (f *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: opOpen, Path: name, Err: fs.ErrInvalid}
	}
	e, ok := f.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: opOpen, Path: name, Err: fs.ErrNotExist}
	}
	if e.dir {
		return &dirHandle{fsys: f, path: name, entry: e}, nil
	}
	data, err := f.bytesOf(e)
	if err != nil {
		return nil, &fs.PathError{Op: opOpen, Path: name, Err: err}
	}
	return &fileHandle{info: f.statOf(name, e), r: bytes.NewReader(data)}, nil
}

// bytesOf hands back an entry's file bytes, fetching an attachment's blob
// exactly once.
func (f *FS) bytesOf(e *entry) ([]byte, error) {
	if e.sha == "" {
		return e.data, nil
	}
	e.once.Do(func() {
		e.blob, e.blobErr = f.fetchBlob(e.sha)
	})
	if e.blobErr != nil {
		return nil, fmt.Errorf("%w: %w", fs.ErrNotExist, e.blobErr)
	}
	return e.blob, nil
}

// statOf forms an entry's FileInfo. The root is named for the vault; an
// attachment's size is its materialized blob's, so stating one fetches it.
func (f *FS) statOf(name string, e *entry) fileInfo {
	base := path.Base(name)
	if name == "." {
		base = f.name
	}
	info := fileInfo{name: base, mod: e.mod, mode: 0o444}
	switch {
	case e.dir:
		info.mode = fs.ModeDir | 0o555
	case e.sha != "":
		info.size = int64(len(e.blob))
	default:
		info.size = int64(len(e.data))
	}
	return info
}

// fetchBlob tries the vault's stores in preference order with the bare-hash
// GET every Blossom server accepts (BUD-01). The winning body must hash to
// the stated identity; a mismatch is a miss and the next store gets its
// turn.
func (f *FS) fetchBlob(sha string) ([]byte, error) {
	if len(f.servers) == 0 {
		return nil, errors.New("the vault names no blob store")
	}
	var lastErr error
	for _, server := range f.servers {
		data, err := f.fetchFrom(strings.TrimRight(server, "/")+"/"+sha, sha)
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// fetchFrom performs one store request and verifies the bytes against the
// stated hash.
func (f *FS) fetchFrom(url, sha string) (data []byte, rerr error) {
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building the blob request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching the blob: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && rerr == nil {
			rerr = fmt.Errorf("closing the blob response: %w", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the store answered %d for %s", resp.StatusCode, url)
	}
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the blob: %w", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != sha {
		return nil, fmt.Errorf("the store's bytes hash to %s, not the stated %s", got, sha)
	}
	return data, nil
}

// fileInfo is the FileInfo every handle and entry reports.
type fileInfo struct {
	// mod leads for field alignment: its location pointer is the struct's
	// last pointer word this way.
	mod  time.Time
	name string
	size int64
	mode fs.FileMode
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return i.size }
func (i fileInfo) Mode() fs.FileMode  { return i.mode }
func (i fileInfo) ModTime() time.Time { return i.mod }
func (i fileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fileInfo) Sys() any           { return nil }

// fileHandle serves a materialized file: a seekable reader over bytes, which
// is what http.ServeFileFS needs.
type fileHandle struct {
	r    *bytes.Reader
	info fileInfo
}

func (h *fileHandle) Stat() (fs.FileInfo, error) { return h.info, nil }
func (h *fileHandle) Read(p []byte) (int, error) { return h.r.Read(p) }
func (h *fileHandle) Close() error               { return nil }

func (h *fileHandle) Seek(offset int64, whence int) (int64, error) {
	return h.r.Seek(offset, whence)
}

// dirHandle serves a directory as fs.ReadDirFile.
type dirHandle struct {
	fsys  *FS
	entry *entry
	path  string
	pos   int
}

func (h *dirHandle) Stat() (fs.FileInfo, error) { return h.fsys.statOf(h.path, h.entry), nil }
func (h *dirHandle) Close() error               { return nil }

func (h *dirHandle) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: h.path, Err: errors.New("is a directory")}
}

func (h *dirHandle) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := h.entry.children[h.pos:]
	if n <= 0 {
		h.pos = len(h.entry.children)
		return h.fsys.dirEntries(h.path, rest), nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	h.pos += n
	return h.fsys.dirEntries(h.path, rest[:n]), nil
}

func (f *FS) dirEntries(dir string, names []string) []fs.DirEntry {
	if dir == "." {
		dir = ""
	}
	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, &dirEntry{fsys: f, name: name, full: joinDirPath(dir, name)})
	}
	return entries
}

// dirEntry names one child of a directory. Info materializes an attachment's
// blob so the reported size and the opened file agree.
type dirEntry struct {
	fsys *FS
	name string
	full string
}

func (d *dirEntry) Name() string { return d.name }

func (d *dirEntry) IsDir() bool { return d.Type().IsDir() }

func (d *dirEntry) Type() fs.FileMode {
	if e, ok := d.fsys.entries[d.full]; ok && e.dir {
		return fs.ModeDir
	}
	return 0
}

func (d *dirEntry) Info() (fs.FileInfo, error) {
	e, ok := d.fsys.entries[d.full]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: d.full, Err: fs.ErrNotExist}
	}
	if e.sha != "" {
		if _, err := d.fsys.bytesOf(e); err != nil {
			return nil, &fs.PathError{Op: "stat", Path: d.full, Err: err}
		}
	}
	return d.fsys.statOf(d.full, e), nil
}
