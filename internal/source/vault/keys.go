package vault

import (
	"path"
	"strings"
)

// DocDTag maps an in-mount document path back to its d-tag, the inverse of
// okf.DTagToPath. The handler seeds each Resolve call with it, so the
// referencing document's location drives doc-relative matching. The path is
// bundle-relative and carries the concept extension.
func (v *Vault) DocDTag(relPath string) (string, error) {
	return v.Domain.MemberDTag(strings.TrimSuffix(path.Clean(relPath), conceptExt))
}
