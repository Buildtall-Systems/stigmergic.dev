package provenance

import (
	"cmp"
	"slices"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Row is one feed in a ranking. Feed and Via are hex; Via is empty for a
// feed no record names as a subject.
type Row struct {
	Feed    string `json:"feed"`
	Via     string `json:"via,omitempty"`
	Score   int    `json:"score"`
	Useful  int    `json:"useful"`
	Saves   int    `json:"saves"`
	Listens int    `json:"listens"`
}

// Rank scores every feed by the useful content it led to. An article is
// useful when it was saved or its listen completed; one with both counts
// once. A feed's score is the count of distinct useful articles across the
// feed itself and every feed reachable from it through records, each feed
// counted once, so a cycle of records adds nothing twice. Saved and listened
// are kind 30023 coordinates; any other coordinate is ignored.
func Rank(records []Record, saved, listened []string) []Row {
	rows := map[string]*Row{}
	row := func(feed string) *Row {
		if r, ok := rows[feed]; ok {
			return r
		}
		r := &Row{Feed: feed}
		rows[feed] = r
		return r
	}

	useful := map[string]bool{}
	count := func(coords []string, field func(*Row) *int) {
		seen := map[string]bool{}
		for _, coord := range coords {
			feed, ok := articleFeed(coord)
			if !ok || seen[coord] {
				continue
			}
			seen[coord] = true
			r := row(feed)
			*field(r)++
			if !useful[coord] {
				useful[coord] = true
				r.Useful++
			}
		}
	}
	count(saved, func(r *Row) *int { return &r.Saves })
	count(listened, func(r *Row) *int { return &r.Listens })

	children := map[string][]string{}
	for _, rec := range records {
		via := rec.ReferringFeed()
		if via == "" {
			continue
		}
		row(rec.Subject).Via = via
		row(via)
		children[via] = append(children[via], rec.Subject)
	}

	out := make([]Row, 0, len(rows))
	for feed, r := range rows {
		r.Score = closure(feed, children, rows)
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Row) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(b.Useful, a.Useful), cmp.Compare(a.Feed, b.Feed))
	})
	return out
}

func closure(feed string, children map[string][]string, rows map[string]*Row) int {
	visited := map[string]bool{feed: true}
	stack := []string{feed}
	total := 0
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		total += rows[f].Useful
		for _, child := range children[f] {
			if !visited[child] {
				visited[child] = true
				stack = append(stack, child)
			}
		}
	}
	return total
}

func articleFeed(coord string) (string, bool) {
	kind, pubkey, dTag, err := btknostr.ParseCoordinate(coord)
	if err != nil || kind != btknostr.KindLongForm || pubkey == "" || dTag == "" {
		return "", false
	}
	return pubkey, true
}
