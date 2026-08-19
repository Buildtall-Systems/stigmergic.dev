package vault

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testFS(t *testing.T, base string) *FS {
	t.Helper()
	v, _ := testVault(t, base)
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, v.docModTimes(), v.Servers, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return fsys
}

func TestFSConformance(t *testing.T) {
	fsys := testFS(t, testStore(t).URL)
	if err := fstest.TestFS(fsys,
		cidFirst+conceptExt,
		cidSecond+conceptExt,
		cidGuide+conceptExt,
		dirThoughts+"/"+nameArt,
		nameCover,
	); err != nil {
		t.Fatalf("fstest.TestFS: %v", err)
	}
}

func TestFSRootIsNamedForTheVault(t *testing.T) {
	info, err := fs.Stat(testFS(t, ""), ".")
	if err != nil {
		t.Fatalf("Stat(.): %v", err)
	}
	if !info.IsDir() || info.Name() != testVaultName {
		t.Errorf("root stat = %q dir=%t, want the directory %q", info.Name(), info.IsDir(), testVaultName)
	}
}

func TestFSConceptCarriesTheDocumentTime(t *testing.T) {
	info, err := fs.Stat(testFS(t, ""), cidFirst+conceptExt)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.ModTime().Equal(testPublishTime()) {
		t.Errorf("mod time = %v, want the document event's %v", info.ModTime(), testPublishTime())
	}
	if info.Size() == 0 {
		t.Error("size = 0, want the serialized concept's byte length")
	}
}

func TestFSBlobVerifiesTheStatedHash(t *testing.T) {
	data, err := fs.ReadFile(testFS(t, testStore(t).URL), dirThoughts+"/"+nameArt)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(artBytes()) {
		t.Errorf("blob bytes = %q, want %q", data, artBytes())
	}
}

func TestFSBlobMismatchIsNotFound(t *testing.T) {
	lying := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("wrong bytes")); err != nil {
			t.Errorf("writing the blob: %v", err)
		}
	}))
	t.Cleanup(lying.Close)

	_, err := fs.ReadFile(testFS(t, lying.URL), dirThoughts+"/"+nameArt)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist for a hash mismatch", err)
	}
}

func TestFSNoStoreIsNotFound(t *testing.T) {
	_, err := fs.ReadFile(testFS(t, ""), dirThoughts+"/"+nameArt)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist when the vault names no store", err)
	}
}
