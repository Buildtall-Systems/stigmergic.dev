package okf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// A bundle's directory names which vault it is, since under projection D the
// root directory is the root d-tag verbatim. It does not name whose vault it
// is, so identity has come entirely from whatever configuration happened to be
// ambient when a verb ran, and publishing the right bundle against the wrong
// configuration republishes the whole vault under the wrong owner, with sets
// referencing coordinates nothing answers. The vault-local file is where a
// bundle says whose it is, so a publish can refuse that before it opens a
// relay.
//
// It states where a key is found and never a key. A bundle is a directory tree
// a person edits and git tracks, so a secret written into it is a secret
// committed by the next person who was not thinking about it.

const (
	// VaultConfigFileName is the reserved file at the bundle root stating whose
	// vault the bundle holds. Only the root states it: a bundle has one owner,
	// and a subdirectory stating a second would be a second answer to a
	// question with one, whichever answer lost losing silently.
	//
	// It is a dotfile for the same reason the sidecar is: a renderer walking
	// the bundle passes over it without being taught to.
	VaultConfigFileName = "." + vaultConfigBase + vaultConfigExt

	// VaultOwnerKey holds the npub whose vault the bundle is. It is the fact
	// the file exists to carry, so a file stating no owner is refused.
	VaultOwnerKey = "owner"

	// VaultNsecSourceKey names where that owner's key is found, as a location
	// and never as a key.
	VaultNsecSourceKey = "nsec_source"

	// NsecSourceEnvScheme names an environment variable holding the key, as
	// env:NAME.
	NsecSourceEnvScheme = "env"

	// vaultConfigBase and the two extensions compose the reserved name and the
	// near-miss set from one spelling, so the two cannot drift apart.
	vaultConfigBase   = "vault"
	vaultConfigExt    = ".yaml"
	vaultConfigAltExt = ".yml"

	nsecSourceSeparator = ":"
)

// VaultConfig is what a bundle states about itself that its tree cannot: whose
// vault it holds, and where that owner's key is found.
type VaultConfig struct {
	// Owner is the npub the bundle belongs to. A publish signs as this
	// identity or refuses to sign at all.
	Owner string
	// NsecSource names where the owner's key lives. Empty means the bundle
	// makes no claim and the key comes from configuration, which is what a
	// bundle exported before this file existed looks like.
	NsecSource string
}

// Validate refuses a vault config that cannot do the one job it has. The owner
// is decoded rather than shape-checked, for the same reason the configuration
// layer decodes it: an npub that will not decode otherwise fails much later, at
// a protocol boundary where the message says far less about what went wrong.
func (v VaultConfig) Validate() error {
	if v.Owner == "" {
		return fmt.Errorf("%s states no %s: the file exists to name the vault's owner, and one naming nobody leaves the owner to whatever configuration happens to be ambient",
			VaultConfigFileName, VaultOwnerKey)
	}
	if _, err := btknostr.NpubToHex(v.Owner); err != nil {
		return fmt.Errorf("%s: %s: %w", VaultConfigFileName, VaultOwnerKey, err)
	}
	if v.NsecSource == "" {
		return nil
	}
	return CheckNsecSource(v.NsecSource)
}

// CheckNsecSource admits a key source this format resolves and refuses one it
// does not, rather than letting an unrecognised spelling fall through to
// whatever key configuration happened to supply. The scheme is what says which
// kind of location the rest names.
//
// env is the only scheme, and it is the one needed: secretspec, the other place
// a buildtall key lives, injects into the environment of the process it runs,
// so a secretspec-managed key arrives as an environment variable and env names
// it. Refusing an unknown scheme rather than falling through is what keeps a
// second scheme additive.
func CheckNsecSource(source string) error {
	scheme, name, found := strings.Cut(source, nsecSourceSeparator)
	if !found {
		return fmt.Errorf("%s: %s %q states no scheme: write %s%sNAME to name an environment variable",
			VaultConfigFileName, VaultNsecSourceKey, source, NsecSourceEnvScheme, nsecSourceSeparator)
	}
	if scheme != NsecSourceEnvScheme {
		return fmt.Errorf("%s: %s %q names the scheme %q, which this format does not resolve; %q is the scheme it resolves",
			VaultConfigFileName, VaultNsecSourceKey, source, scheme, NsecSourceEnvScheme)
	}
	if name == "" {
		return fmt.Errorf("%s: %s %q names no variable", VaultConfigFileName, VaultNsecSourceKey, source)
	}
	return nil
}

// EnvNsecSource composes the key source naming an environment variable. A
// caller stating one composes it here rather than spelling the scheme out, so
// what is written can never drift from what CheckNsecSource admits.
func EnvNsecSource(name string) string {
	return NsecSourceEnvScheme + nsecSourceSeparator + name
}

// ParseVaultConfig reads vault config bytes. A key the format does not define
// is refused rather than dropped, the same posture ParseNode takes and for the
// same reason: a misspelled key would otherwise be silent, and silence here
// returns the bundle to the ambient configuration this file exists to displace.
func ParseVaultConfig(data []byte) (*VaultConfig, error) {
	var raw struct {
		Owner      string `yaml:"owner"`
		NsecSource string `yaml:"nsec_source"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty file decodes to EOF, and unlike the sidecar that is not a shape
	// this format has a meaning for: Validate refuses it by way of the absent
	// owner, which is the message a person emptying the file needs to read.
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", VaultConfigFileName, err)
	}

	v := &VaultConfig{Owner: raw.Owner, NsecSource: raw.NsecSource}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return v, nil
}

// WriteVaultConfig serializes vault config to file bytes: the owner first,
// since it is the fact the file is read for, then the key source where there is
// one.
func WriteVaultConfig(v VaultConfig) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}

	m := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, strScalar(VaultOwnerKey), strScalar(v.Owner))
	if v.NsecSource != "" {
		m.Content = append(m.Content, strScalar(VaultNsecSourceKey), strScalar(v.NsecSource))
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("okf: marshaling %s: %w", VaultConfigFileName, err)
	}
	return data, nil
}

// readVaultConfig reads the file at diskPath, naming it in every failure so the
// message distinguishes it from the sidecar a directory may also carry.
func readVaultConfig(diskPath string) (*VaultConfig, error) {
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return nil, fmt.Errorf("okf: reading %q: %w", diskPath, err)
	}
	v, err := ParseVaultConfig(data)
	if err != nil {
		return nil, fmt.Errorf("okf: %q: %w", diskPath, err)
	}
	return v, nil
}

// isVaultConfigNearMiss reports whether a name is close enough to the reserved
// file to be a misspelling of it. readDirectory otherwise ignores a name it
// does not recognise, on the stated grounds that a bundle tolerates
// attachments,
// and a misspelled vault config would be ignored under exactly that rule. The
// bundle would then fall back to ambient configuration for its owner, which is
// the failure the file exists to prevent, and it would do so in silence.
//
// The rule is enumerable rather than a similarity measure, so it can be stated
// and tested: the reserved base name in any casing, with the leading dot
// optional and either YAML extension.
func isVaultConfigNearMiss(name string) bool {
	if name == VaultConfigFileName {
		return false
	}
	trimmed := strings.TrimPrefix(strings.ToLower(name), ".")
	return trimmed == vaultConfigBase+vaultConfigExt || trimmed == vaultConfigBase+vaultConfigAltExt
}
