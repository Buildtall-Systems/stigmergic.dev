package models

// UICapabilities gates UI features on what the content source supports.
// Each flag maps to a source capability: RecentlyUpdated to meaningful mod
// times, GitignoreToggle to runtime gitignore awareness, CopyPath to a local
// filesystem root.
type UICapabilities struct {
	RecentlyUpdated bool
	GitignoreToggle bool
	CopyPath        bool
}
