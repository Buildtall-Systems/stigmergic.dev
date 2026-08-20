package models

// VaultEntry is one row of the sidebar's vault panel: the name the vault
// gave itself, the npub that published it, and the route prefix its tree
// answers under. Owner is the whole npub, which is what the row's title
// attribute carries; the label beside the name is the shortened form.
type VaultEntry struct {
	Name  string
	Owner string
	Mount string
}

// ShortOwner is the display form of the owner npub: enough of the head and
// tail to recognize a vault's publisher, never enough to retype one. The
// full value travels in the row's title attribute, so shortening the label
// loses nothing.
func (v VaultEntry) ShortOwner() string {
	return ShortNpub(v.Owner)
}

// npubHead and npubTail are how much of an npub a shortened label keeps.
// The head includes the "npub1" prefix, so the recognizable part is what
// follows it.
const (
	npubHead = 12
	npubTail = 6
)

// ShortNpub abbreviates an npub for display. A value too short to abbreviate
// is returned whole rather than mangled into something longer than it began.
func ShortNpub(npub string) string {
	if len(npub) <= npubHead+npubTail+1 {
		return npub
	}
	return npub[:npubHead] + "…" + npub[len(npub)-npubTail:]
}

// SidebarView is one render of the left panel: the primary source's tree
// above, and one row per mounted vault below. The two halves travel together
// because every page render draws the panel whole, and because only one of
// them describes the source the reader started the server on.
type SidebarView struct {
	Primary TreeView
	Vaults  []VaultEntry
}
