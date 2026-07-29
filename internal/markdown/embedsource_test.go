package markdown

import (
	"testing"
	"testing/fstest"
)

const (
	testAttachmentRoot = "file"
	testImageName      = "dependency.png"
	testImagePath      = testAttachmentRoot + "/" + testImageName
	testSVGPath        = "diagrams/flow.svg"
	testNotePath       = "reading/papers/lifes irreducible structure.md"
	testNoteBody       = "## DNA\n\nbody\n"
)

func testVaultFS() fstest.MapFS {
	return fstest.MapFS{
		testNotePath:  {Data: []byte(testNoteBody)},
		testImagePath: {Data: []byte("\x89PNG")},
		testSVGPath:   {Data: []byte("<svg/>")},
		"secrets.md":  {Data: []byte("top level\n")},
	}
}

func TestFSEmbedSourceNoteSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		notePath string
		wantBody string
		wantOK   bool
	}{
		{
			name:     "note read hit",
			notePath: testNotePath,
			wantOK:   true,
			wantBody: testNoteBody,
		},
		{
			name:     "note read miss",
			notePath: "reading/papers/absent.md",
			wantOK:   false,
		},
		{
			name:     "traversal rejected",
			notePath: "../secrets.md",
			wantOK:   false,
		},
		{
			name:     "absolute path rejected",
			notePath: "/secrets.md",
			wantOK:   false,
		},
		{
			name:     "directory is not a note",
			notePath: "reading/papers",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := NewFSEmbedSource(testVaultFS(), "")
			got, ok := src.NoteSource(tt.notePath)
			if ok != tt.wantOK {
				t.Fatalf("expected ok %v, got %v", tt.wantOK, ok)
			}
			if ok && string(got) != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, got)
			}
		})
	}
}

func TestFSEmbedSourceProbeAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		attachmentRoot string
		target         string
		wantPath       string
		wantOK         bool
	}{
		{
			name:     "hit at content root with an explicit path",
			target:   testSVGPath,
			wantOK:   true,
			wantPath: testSVGPath,
		},
		{
			name:           "bare filename found only via the attachment root",
			attachmentRoot: testAttachmentRoot,
			target:         testImageName,
			wantOK:         true,
			wantPath:       testImagePath,
		},
		{
			name:   "bare filename misses without an attachment root",
			target: testImageName,
			wantOK: false,
		},
		{
			name:           "content root wins over the attachment root",
			attachmentRoot: testAttachmentRoot,
			target:         testSVGPath,
			wantOK:         true,
			wantPath:       testSVGPath,
		},
		{
			name:           "probe miss",
			attachmentRoot: testAttachmentRoot,
			target:         "absent.png",
			wantOK:         false,
		},
		{
			name:   "traversal rejected",
			target: "../../etc/passwd",
			wantOK: false,
		},
		{
			name:           "traversal through the attachment root rejected",
			attachmentRoot: testAttachmentRoot,
			target:         "../../secrets.md",
			wantOK:         false,
		},
		{
			name:   "directory is not an asset",
			target: "diagrams",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := NewFSEmbedSource(testVaultFS(), tt.attachmentRoot)
			got, ok := src.ProbeAsset(tt.target)
			if ok != tt.wantOK {
				t.Fatalf("expected ok %v, got %v (path %q)", tt.wantOK, ok, got)
			}
			if ok && got != tt.wantPath {
				t.Errorf("expected path %q, got %q", tt.wantPath, got)
			}
		})
	}
}
