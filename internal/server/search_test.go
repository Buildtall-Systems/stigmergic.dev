package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

const searchNotesFile = "notes.md"

func testSearchIndex() searchIndex {
	contents := map[string][]byte{
		searchNotesFile: []byte("# Notes\n\nThe freshness protocol tracks per-relay event recency across the fleet.\n"),
		"docs/upper.md": []byte("# Upper\n\nFRESHNESS in capitals here.\n"),
		"plain.md":      []byte("nothing relevant\n"),
	}
	files := []models.SearchableFile{
		{Path: "/" + searchNotesFile, Name: searchNotesFile},
		{Path: "/docs/upper.md", Name: "upper.md"},
		{Path: "/plain.md", Name: "plain.md"},
	}
	return buildSearchIndex(contents, files)
}

func TestSearchMatchesAndSnippets(t *testing.T) {
	t.Parallel()

	resp := testSearchIndex().search("freshness protocol", 20)

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(resp.Results), resp.Results)
	}
	m := resp.Results[0]
	if m.Path != "/"+searchNotesFile || m.Title != searchNotesFile {
		t.Errorf("unexpected result identity: %+v", m)
	}
	got := m.Snippet[m.MatchStart:m.MatchEnd]
	if got != "freshness protocol" {
		t.Errorf("match offsets select %q, want the query text", got)
	}
	if strings.Contains(m.Snippet, "\n") {
		t.Error("snippet must have line breaks flattened")
	}
	if resp.Truncated {
		t.Error("expected no truncation")
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	t.Parallel()

	resp := testSearchIndex().search("Freshness", 20)

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results across cases, got %d", len(resp.Results))
	}
}

func TestSearchSnippetAtDocumentBoundaries(t *testing.T) {
	t.Parallel()

	contents := map[string][]byte{
		"tiny.md": []byte("edge"),
	}
	files := []models.SearchableFile{{Path: "/tiny.md", Name: "tiny.md"}}

	resp := buildSearchIndex(contents, files).search("edge", 20)

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	m := resp.Results[0]
	if m.Snippet != "edge" {
		t.Errorf("expected whole-document snippet, got %q", m.Snippet)
	}
	if m.MatchStart != 0 || m.MatchEnd != 4 {
		t.Errorf("expected offsets [0,4], got [%d,%d]", m.MatchStart, m.MatchEnd)
	}
}

func TestSearchResultCapAndTruncation(t *testing.T) {
	t.Parallel()

	contents := make(map[string][]byte)
	files := make([]models.SearchableFile, 0, 25)
	for i := range 25 {
		route := fmt.Sprintf("doc%02d.md", i)
		contents[route] = []byte("shared target phrase\n")
		files = append(files, models.SearchableFile{Path: "/" + route, Name: route})
	}

	resp := buildSearchIndex(contents, files).search("target", 20)

	if len(resp.Results) != 20 {
		t.Errorf("expected results capped at 20, got %d", len(resp.Results))
	}
	if !resp.Truncated {
		t.Error("expected truncation flag")
	}
}

func TestSearchEmptyQueryAndNoMatch(t *testing.T) {
	t.Parallel()

	idx := testSearchIndex()

	for _, q := range []string{"", "   "} {
		resp := idx.search(q, 20)
		if len(resp.Results) != 0 || resp.Truncated {
			t.Errorf("query %q: expected empty results, got %+v", q, resp)
		}
	}

	resp := idx.search("zxqv-not-present", 20)
	if len(resp.Results) != 0 {
		t.Errorf("expected no matches, got %d", len(resp.Results))
	}
}

func searchViaAPI(t *testing.T, port int, query string) searchResponse {
	t.Helper()
	u := fmt.Sprintf("http://localhost:%d/api/search?q=%s", port, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close response body: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	var out searchResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("search response is not valid JSON: %v (body %q)", err, string(b))
	}
	return out
}

func TestSearchAPIFilesystemSource(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "research.md", "# Research\n\nThe stigmergic trace mediates coordination.\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	deadline := time.Now().Add(5 * time.Second)
	var out searchResponse
	for time.Now().Before(deadline) {
		out = searchViaAPI(t, port, "stigmergic trace")
		if len(out.Results) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(out.Results) != 1 {
		t.Fatalf("expected 1 result once indexed, got %+v", out)
	}
	if out.Results[0].Path != "/research.md" {
		t.Errorf("expected /research.md, got %s", out.Results[0].Path)
	}
}

func TestSearchAPIEmbeddedSource(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	out := searchViaAPI(t, port, "embedded home")

	if len(out.Results) != 1 {
		t.Fatalf("expected 1 result from embedded corpus, got %+v", out)
	}
	if out.Results[0].Path != "/"+testIndexFile {
		t.Errorf("expected /%s, got %s", testIndexFile, out.Results[0].Path)
	}
}

func TestSearchIndexRebuildOnChange(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "first.md", "# First\n\noriginal wording\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	testutil.CreateTestFile(t, dir, "second.md", "# Second\n\nfreshly added content\n")

	deadline := time.Now().Add(5 * time.Second)
	var out searchResponse
	for time.Now().Before(deadline) {
		out = searchViaAPI(t, port, "freshly added")
		if len(out.Results) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(out.Results) != 1 {
		t.Fatalf("expected new file's content to be searchable after rescan, got %+v", out)
	}
	if out.Results[0].Path != "/second.md" {
		t.Errorf("expected /second.md, got %s", out.Results[0].Path)
	}
}
