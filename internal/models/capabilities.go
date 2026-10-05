package models

// UICapabilities gates UI features on what the content source supports.
// Each flag maps to a source capability: RecentlyUpdated to meaningful mod
// times, GitignoreToggle to runtime gitignore awareness, CopyPath to a local
// filesystem root, FollowMode to live change events. Login belongs to the
// server, not a source: it is set when the btk login routes are mounted.
type UICapabilities struct {
	RecentlyUpdated bool
	GitignoreToggle bool
	CopyPath        bool
	FollowMode      bool
	Login           bool
}
