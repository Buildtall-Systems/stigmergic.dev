package vault

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

// EventSink is the narrow slice of *nostr.SimplePool that publishing
// exercises. Extracting it lets the order and the abort be tested without a
// relay; *nostr.SimplePool satisfies it directly.
type EventSink interface {
	PublishMany(ctx context.Context, urls []string, evt nostr.Event) chan nostr.PublishResult
}

// NameFromBundle reads which vault a bundle is. Under projection D a member's
// path is its d-tag verbatim, so the bundle's own directory is the root d-tag
// and the tree on disk says where it belongs. No flag can contradict it, which
// is what keeps a bundle from being published into a vault it was never
// exported from.
func NameFromBundle(b *okf.Bundle) (string, error) {
	name, class := lists.ClassifyVaultDTag(b.Name)
	if class != lists.DTagRoot {
		return "", fmt.Errorf(
			"bundle directory %q does not name a vault root: a bundle's directory is its root d-tag, %q followed by the vault name",
			b.Name, lists.VaultFamilyPrefix)
	}
	return name, nil
}

// CheckBundleVersion refuses a bundle this exporter did not write. The bundle
// is authoritative for the whole tag list, so a publish reconstructs each event
// from the bundle alone: a tag the bundle does not state is a tag the publish
// deletes. A bundle written before the format stated every tag therefore does
// not leave the tags it cannot express alone, it erases them, and the erasure
// cannot be recalled. The version is what tells a bundle that states nothing
// apart from one that has nothing to state.
//
// The check runs before a relay is touched, and before the key is looked at, so
// a stale bundle is refused whether or not a dry run was asked for.
func CheckBundleVersion(b *okf.Bundle) error {
	got := b.Root.Node.FormatVersion
	switch {
	case got == okf.BundleFormatVersion:
		return nil
	case got > okf.BundleFormatVersion:
		return fmt.Errorf(
			"bundle states format version %d, but this exporter writes %d: upgrade btcli rather than publishing a bundle it cannot fully read",
			got, okf.BundleFormatVersion)
	default:
		return fmt.Errorf(
			"bundle states no format version this exporter accepts (found %d, want %d): re-export it with `btcli vault export` before publishing, since publishing it now would erase every tag the older format could not state",
			got, okf.BundleFormatVersion)
	}
}

// SigningKey refuses to publish a vault under a key that is not its owner's.
// Signing stamps the signer's own pubkey onto every event, so a key that
// disagrees with the owner does not fail: it republishes the whole vault under
// the wrong identity, with sets referencing coordinates nothing answers.
func SigningKey(nsec, ownerNpub string) (string, error) {
	if nsec == "" {
		return "", errors.New("no key to sign with: set auth.nsec, supply BTCLI_AUTH_NSEC, or pass --nsec with the vault owner's key")
	}
	signer, err := btknostr.NsecToNpub(nsec)
	if err != nil {
		return "", fmt.Errorf("auth.nsec: %w", err)
	}
	if signer != ownerNpub {
		return "", fmt.Errorf(
			"the configured key acts as %s, but the vault being published belongs to %s: publishing would sign every event under the wrong identity",
			signer, ownerNpub)
	}
	return btknostr.NsecToHex(nsec)
}

// Publish signs and publishes in the plan's order, stopping at the first event
// no relay took. The order is the point: a set that references a document which
// never landed is a dangling reference, so an abort leaves the vault short
// rather than inconsistent. Deletions follow everything else, so nothing is
// erased before its replacement exists.
func Publish(ctx context.Context, sink EventSink, relays []string, secHex string, plan *okf.PublishPlan, log *slog.Logger) (published, deleted []*nostr.Event, err error) {
	for _, ev := range plan.Events {
		if err := publishOne(ctx, sink, relays, secHex, ev, log); err != nil {
			return published, deleted, fmt.Errorf("%d events landed, then %w", len(published), err)
		}
		published = append(published, ev)
	}
	for _, ev := range plan.Deletions {
		if err := publishOne(ctx, sink, relays, secHex, ev, log); err != nil {
			return published, deleted, fmt.Errorf("the vault landed, then %w", err)
		}
		deleted = append(deleted, ev)
	}
	return published, deleted, nil
}

// publishOne signs an event and offers it to every relay, accepting the event
// as landed if any one of them took it. A relay that refuses is reported in
// the error only when all of them do, since a vault reaching one relay is
// published and a vault reaching none is not.
func publishOne(ctx context.Context, sink EventSink, relays []string, secHex string, ev *nostr.Event, log *slog.Logger) error {
	if err := ev.Sign(secHex); err != nil {
		return fmt.Errorf("signing %s: %w", EventLabel(ev), err)
	}

	accepted := 0
	var refusals []string
	for res := range sink.PublishMany(ctx, relays, *ev) {
		if res.Error == nil {
			accepted++
			continue
		}
		refusals = append(refusals, fmt.Sprintf("%s: %v", res.RelayURL, res.Error))
	}
	if accepted == 0 {
		return fmt.Errorf("no relay accepted %s: %s", EventLabel(ev), strings.Join(refusals, "; "))
	}

	log.Debug("published", "kind", ev.Kind, "id", ev.ID, "accepted", accepted, "refused", len(refusals))
	return nil
}

// Report is the verb's last word: what was carried to the store and written
// to the relays, in the order it happened, and how much of the vault was left
// alone. Uploads lead because blobs land before the events that state them;
// under dry run the caller passes the plan's uploads, so the would-upload
// count is as honest as the would-publish list. Orphans close the report with
// their remedy spelled out, because no vault verb ever deletes a blob: the
// bytes behind a hash are dissociated only by the operator's own hand.
func Report(w io.Writer, s Subject, plan *okf.PublishPlan, published, deleted []*nostr.Event, uploaded []okf.BlobUpload, dryRun bool) error {
	head := fmt.Sprintf("published %s as %s", s.Domain.RootDTag, s.Owner)
	if dryRun {
		head = fmt.Sprintf("would publish %s as %s (dry run)", s.Domain.RootDTag, s.Owner)
	}
	if _, err := fmt.Fprintln(w, head); err != nil {
		return err
	}

	for _, u := range uploaded {
		if _, err := fmt.Fprintf(w, "  upload %s %s\n", u.Path, u.SHA256); err != nil {
			return err
		}
	}
	for _, ev := range published {
		if _, err := fmt.Fprintf(w, "  %s\n", EventLabel(ev)); err != nil {
			return err
		}
	}
	for _, ev := range deleted {
		if _, err := fmt.Fprintf(w, "  delete %s\n", EventLabel(ev)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "  %d events, %d deletions, %d uploads, %d unchanged\n",
		len(published), len(deleted), len(uploaded), len(plan.Unchanged)); err != nil {
		return err
	}

	if len(plan.Orphans) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "  orphaned blobs: %d stated by no directory; this verb deletes none\n", len(plan.Orphans)); err != nil {
		return err
	}
	for _, hash := range plan.Orphans {
		if _, err := fmt.Fprintf(w, "    btcli blossom delete %s\n", hash); err != nil {
			return err
		}
	}
	return nil
}

// EventLabel names an event the way an operator reads a vault: by the d-tag it
// addresses. A deletion carries no d-tag of its own, so it is named by the
// coordinate it erases.
func EventLabel(ev *nostr.Event) string {
	if ev.Kind == nostr.KindDeletion {
		if tag := ev.Tags.Find("a"); tag != nil {
			return fmt.Sprintf("kind %d of %s", ev.Kind, tag[1])
		}
		return fmt.Sprintf("kind %d", ev.Kind)
	}
	return fmt.Sprintf("kind %d %s", ev.Kind, lists.GetDTag(ev))
}
