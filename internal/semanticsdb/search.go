package semanticsdb

import (
	"sort"
	"strings"
)

// SearchQuery narrows the action search. Empty fields match anything.
type SearchQuery struct {
	Name        string
	StructName  string
	PacketID    string
	OpenkoreName string
	Description string
}

// Search returns actions matching all non-empty query fields. Matching is
// case-insensitive substring. Returns at most `limit` results, or all if
// limit <= 0.
func (d *DB) Search(q SearchQuery, limit int) []Action {
	if limit <= 0 {
		limit = -1
	}
	nameSub := strings.ToLower(strings.TrimSpace(q.Name))
	structSub := strings.ToLower(strings.TrimSpace(q.StructName))
	pidSub := normPacketID(q.PacketID)
	openkoreSub := strings.ToLower(strings.TrimSpace(q.OpenkoreName))
	descSub := strings.ToLower(strings.TrimSpace(q.Description))

	var matches []Action
	for _, name := range d.ListActions() {
		a, _ := d.GetAction(name)
		if nameSub != "" && !strings.Contains(strings.ToLower(a.Name), nameSub) {
			continue
		}
		if openkoreSub != "" && !strings.Contains(strings.ToLower(a.OpenkoreName), openkoreSub) {
			continue
		}
		if descSub != "" && !strings.Contains(strings.ToLower(a.Description), descSub) {
			continue
		}
		if structSub != "" || pidSub != "" {
			matchedAny := false
			for _, impl := range a.Implementations {
				if structSub != "" && !strings.Contains(strings.ToLower(impl.StructName), structSub) {
					continue
				}
				if pidSub != "" && impl.PacketID != pidSub {
					continue
				}
				matchedAny = true
				break
			}
			if !matchedAny {
				continue
			}
		}
		matches = append(matches, a)
		if limit > 0 && len(matches) >= limit {
			break
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}
