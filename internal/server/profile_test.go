package server

import (
	"context"
	"errors"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// stubProfiles answers every lookup with one profile and error.
type stubProfiles struct {
	profile *btknostr.Profile
	err     error
	calls   int
}

func (s *stubProfiles) Resolve(context.Context, string) (*btknostr.Profile, error) {
	s.calls++
	return s.profile, s.err
}

// TestResolveUserFillsTheAvatar covers the nav identity btk's avatar dropdown
// renders: a found kind 0 profile gives it a name and picture, and every
// other outcome leaves the npub alone, which the dropdown shows as a letter.
func TestResolveUserFillsTheAvatar(t *testing.T) {
	t.Parallel()

	const picture = "https://example.com/alice.png"
	found := &btknostr.Profile{
		Event:       &nostr.Event{Kind: nostr.KindProfileMetadata},
		Npub:        aliceNpub,
		Name:        "alice",
		DisplayName: "Alice",
		Picture:     picture,
	}

	cases := []struct {
		name        string
		stub        *stubProfiles
		wantName    string
		wantPicture string
	}{
		{name: "found profile", stub: &stubProfiles{profile: found}, wantName: "Alice", wantPicture: picture},
		{name: "lookup error", stub: &stubProfiles{err: errors.New("relays unreachable")}},
		{name: "no profile", stub: &stubProfiles{}},
		{name: "no kind 0 found", stub: &stubProfiles{profile: &btknostr.Profile{Npub: aliceNpub, Name: "npub1paydacp4…p2vp"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			user := resolveUser(tc.stub)(t.Context(), aliceNpub)
			if user == nil {
				t.Fatal("resolveUser returned nil, which btk forbids")
			}
			if user.Npub != aliceNpub {
				t.Errorf("expected npub %s, got %s", aliceNpub, user.Npub)
			}
			if user.DisplayName != tc.wantName {
				t.Errorf("expected display name %q, got %q", tc.wantName, user.DisplayName)
			}
			if user.Picture != tc.wantPicture {
				t.Errorf("expected picture %q, got %q", tc.wantPicture, user.Picture)
			}
			if tc.stub.calls != 1 {
				t.Errorf("expected one lookup, got %d", tc.stub.calls)
			}
		})
	}
}

// TestResolveUserWithoutSource is the lookup turned off: an empty profile
// relay list gives no source, and the nav shows the npub alone.
func TestResolveUserWithoutSource(t *testing.T) {
	t.Parallel()

	if newProfileSource(nil) != nil {
		t.Fatal("an empty relay list built a profile source")
	}
	user := resolveUser(nil)(t.Context(), aliceNpub)
	if user == nil || user.Npub != aliceNpub || user.Picture != "" || user.DisplayName != "" {
		t.Errorf("expected the bare npub, got %+v", user)
	}
}
