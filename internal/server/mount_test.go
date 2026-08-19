package server

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
)

const (
	testVaultMount = "/vault/npub1example/notes/"
	testOwnerNpub  = "npub1example"
)

// timestampedSource is a source with meaningful mod times and nothing else,
// the shape a fetched vault presents: no events to follow, no gitignore to
// toggle, no local path to copy.
type timestampedSource struct {
	fsys fs.FS
	name string
}

var (
	_ source.ContentSource = (*timestampedSource)(nil)
	_ source.Timestamped   = (*timestampedSource)(nil)
)

func (s *timestampedSource) FS() fs.FS           { return s.fsys }
func (s *timestampedSource) Name() string        { return s.name }
func (s *timestampedSource) Close() error        { return nil }
func (s *timestampedSource) ModTimesMeaningful() {}

func TestMountOfPicksTheLongestPrefix(t *testing.T) {
	t.Parallel()

	local := newMount(markdown.FileMount, &timestampedSource{fsys: fstest.MapFS{}, name: "local"}, nil)
	vault := newMount(testVaultMount, &timestampedSource{fsys: fstest.MapFS{}, name: fixtureVaultName}, nil)
	nested := newMount(testVaultMount+"deep/", &timestampedSource{fsys: fstest.MapFS{}, name: "deep"}, nil)
	mounts := []*mount{local, vault, nested}

	tests := []struct {
		name   string
		route  string
		want   *mount
		rel    string
		routed bool
	}{
		{name: "local document", route: markdown.FileMount + "notes/a.md", want: local, rel: "notes/a.md", routed: true},
		{name: "vault document", route: testVaultMount + "thoughts/b.md", want: vault, rel: "thoughts/b.md", routed: true},
		{name: "nested mount wins over the one containing it", route: testVaultMount + "deep/c.md", want: nested, rel: "c.md", routed: true},
		{name: "mount root", route: testVaultMount, want: vault, rel: "", routed: true},
		{name: "another namespace entirely", route: "/api/files", routed: false},
		{name: "a prefix of a mount", route: "/vault/", routed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, rel, ok := mountOf(mounts, tt.route)
			if ok != tt.routed {
				t.Fatalf("mountOf(%q) routed = %v, want %v", tt.route, ok, tt.routed)
			}
			if !tt.routed {
				return
			}
			if got != tt.want {
				t.Errorf("mountOf(%q) chose %q, want %q", tt.route, got.prefix, tt.want.prefix)
			}
			if rel != tt.rel {
				t.Errorf("mountOf(%q) path = %q, want %q", tt.route, rel, tt.rel)
			}
		})
	}
}

// TestVaultMountDeclinesUnroutableNames pins the guard: a name carrying a
// slash would claim paths inside the mount it names.
func TestVaultMountDeclinesUnroutableNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		owner string
		name  string
		want  bool
	}{
		{owner: testOwnerNpub, name: fixtureVaultName, want: true},
		{owner: testOwnerNpub, name: "my notes", want: true},
		{owner: testOwnerNpub, name: "notes/deep", want: false},
		{owner: "npub1x/y", name: fixtureVaultName, want: false},
		{owner: testOwnerNpub, name: "", want: false},
	}

	for _, tt := range tests {
		if got := routable(tt.owner, tt.name); got != tt.want {
			t.Errorf("routable(%q, %q) = %v, want %v", tt.owner, tt.name, got, tt.want)
		}
	}
}

// TestPageCapsGateOnTheSourceServingThePage is the capability contract: what
// acts on the document on screen follows the document, while the sidebar's
// own affordances stay with the primary source.
func TestPageCapsGateOnTheSourceServingThePage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src, err := source.NewFilesystem(dir, false, nil)
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	srv := newServerWithSource(t, src)

	vault := newMount(testVaultMount, &timestampedSource{fsys: fstest.MapFS{}, name: fixtureVaultName}, nil)

	local := srv.pageCaps(srv.primary())
	if !local.CopyPath || !local.FollowMode {
		t.Errorf("local content lost its own affordances: %+v", local)
	}

	fromVault := srv.pageCaps(vault)
	if fromVault.CopyPath {
		t.Error("a vault document offers a path to copy, but it sits at no local path")
	}
	if fromVault.FollowMode {
		t.Error("a vault document offers follow mode, but its source emits no changes")
	}
	if fromVault.GitignoreToggle != local.GitignoreToggle {
		t.Error("the gitignore toggle acts on the primary source and must not follow the document")
	}
	if fromVault.RecentlyUpdated != local.RecentlyUpdated {
		t.Error("the recent list describes the primary source and must not follow the document")
	}
}

func TestRoutedFilesCarryTheirRoutes(t *testing.T) {
	t.Parallel()

	files := []models.SearchableFile{{Name: "a.md", Path: "/a.md"}, {Name: "b.md", Path: "/deep/b.md"}}

	routed := routedFiles(testVaultMount, files)
	if got, want := routed[0].Path, testVaultMount+"a.md"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := routed[1].Path, testVaultMount+"deep/b.md"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if files[0].Path != "/a.md" {
		t.Error("routing rewrote the source's own file list")
	}

	entries := routeEntries(testVaultMount, files)
	if got, want := entries[0].Path, "a.md"; got != want {
		t.Errorf("entry path = %q, want %q: a link writes the name its own source knows", got, want)
	}
	if got, want := entries[0].Route, testVaultMount+"a.md"; got != want {
		t.Errorf("entry route = %q, want %q", got, want)
	}
}
