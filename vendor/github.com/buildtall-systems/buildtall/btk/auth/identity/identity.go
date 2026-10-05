package identity

import (
	"unicode/utf8"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// BunkerBacked says the session signs through a server-held NIP-46 channel
// rather than a browser extension. The nav fragment carries it to the browser
// so the signer facade can route each signing call; window.nostr presence is
// not the same question, because a user with an extension installed may still
// hold a bunker-backed session.
type UserInfo struct {
	Npub         string
	DisplayName  string
	Picture      string
	NIP05        string
	IsAdmin      bool
	BunkerBacked bool
}

type MenuItem struct {
	Label   string
	URL     string
	Visible bool
}

func (u UserInfo) ShortName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return truncateNpub(u.Npub)
}

func (u UserInfo) AvatarLetter() string {
	if u.DisplayName != "" {
		r, _ := utf8.DecodeRuneInString(u.DisplayName)
		if r != utf8.RuneError {
			return string(r)
		}
	}
	stripped := u.Npub
	if len(stripped) > 5 {
		stripped = stripped[5:]
	}
	if stripped != "" {
		r, _ := utf8.DecodeRuneInString(stripped)
		if r != utf8.RuneError {
			return string(r)
		}
	}
	return "?"
}

func FromProfile(p *btknostr.Profile, isAdmin bool) UserInfo {
	if p == nil {
		return UserInfo{}
	}
	displayName := p.DisplayName
	if displayName == "" {
		displayName = p.Name
	}
	return UserInfo{
		Npub:        p.Npub,
		DisplayName: displayName,
		Picture:     p.Picture,
		NIP05:       p.NIP05,
		IsAdmin:     isAdmin,
	}
}

func truncateNpub(npub string) string {
	if len(npub) > 16 {
		return npub[:12] + "…" + npub[len(npub)-4:]
	}
	return npub
}
