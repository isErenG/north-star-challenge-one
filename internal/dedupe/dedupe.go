// Package dedupe finds near-duplicate business records — rows whose name
// and address both strongly resemble another row's, not just rows sharing
// an identifier. True duplicates are merged into their most complete
// member; rows that share a name but sit at a different address are only
// flagged as recurring, since that is normally a chain with several
// branches rather than the same row entered twice.
package dedupe

import (
	"strings"
	"unicode"

	"kbo-review/internal/model"
	"kbo-review/internal/validate"
)

const (
	// nameThreshold and addressThreshold are Jaro-Winkler similarity
	// cutoffs (0-1). Both must be met for two rows to be merged.
	nameThreshold    = 0.92
	addressThreshold = 0.85
	// recurringMax is the address similarity below which a shared name is
	// treated as a different branch rather than a possible duplicate.
	recurringMax = 0.6

	possibleDuplicate = "Possible duplicate"
	recurringName      = "Recurring name, different address"
)

var accents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i",
	"ô", "o", "ö", "o",
	"ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y",
	"&", " en ",
)

// normalize folds a field down to lowercase letters, digits and single
// spaces so formatting differences (accents, punctuation, extra
// whitespace, "&" vs "en") don't hide a near-duplicate.
func normalize(s string) string {
	s = accents.Replace(strings.ToLower(s))
	var b strings.Builder
	space := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case !space:
			b.WriteRune(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// jaro is the Jaro string similarity of two rune slices, in [0, 1].
func jaro(a, b []rune) float64 {
	la, lb := len(a), len(b)
	if la == 0 && lb == 0 {
		return 1
	}
	if la == 0 || lb == 0 {
		return 0
	}
	matchDistance := max(la, lb)/2 - 1
	if matchDistance < 0 {
		matchDistance = 0
	}
	aMatched := make([]bool, la)
	bMatched := make([]bool, lb)
	matches := 0
	for i := range a {
		start, end := i-matchDistance, i+matchDistance+1
		if start < 0 {
			start = 0
		}
		if end > lb {
			end = lb
		}
		for j := start; j < end; j++ {
			if bMatched[j] || a[i] != b[j] {
				continue
			}
			aMatched[i], bMatched[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := range a {
		if !aMatched[i] {
			continue
		}
		for !bMatched[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(la) + m/float64(lb) + (m-float64(transpositions/2))/m) / 3
}

// similarity is Jaro-Winkler: Jaro plus a bonus for a shared prefix, which
// suits short business names and street names better than a plain edit
// distance.
func similarity(x, y string) float64 {
	a, b := []rune(x), []rune(y)
	j := jaro(a, b)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && prefix < 4 && a[prefix] == b[prefix] {
		prefix++
	}
	return j + float64(prefix)*0.1*(1-j)
}

// unionFind groups record indices into duplicate clusters.
type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{p}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// Cluster flags near-duplicate records and merges each cluster into its
// most complete member. Records are only compared within the same
// normalized municipality, which keeps the work proportional to file size
// instead of comparing every row against every other row.
func Cluster(records []model.Record) {
	n := len(records)
	if n < 2 {
		return
	}
	names := make([]string, n)
	addrs := make([]string, n)
	blocks := map[string][]int{}
	for i, r := range records {
		names[i] = normalize(r.Name)
		addrs[i] = normalize(r.Address)
		if names[i] == "" {
			continue
		}
		blocks[normalize(r.Municipality)] = append(blocks[normalize(r.Municipality)], i)
	}

	uf := newUnionFind(n)
	recurring := make([]bool, n)
	for _, group := range blocks {
		for a := 0; a < len(group); a++ {
			for b := a + 1; b < len(group); b++ {
				i, j := group[a], group[b]
				if similarity(names[i], names[j]) < nameThreshold {
					continue
				}
				switch addrSim := similarity(addrs[i], addrs[j]); {
				case addrSim >= addressThreshold:
					uf.union(i, j)
				case addrSim < recurringMax:
					recurring[i], recurring[j] = true, true
				}
			}
		}
	}

	clusters := map[int][]int{}
	for i := range records {
		root := uf.find(i)
		clusters[root] = append(clusters[root], i)
	}
	for _, members := range clusters {
		if len(members) > 1 {
			merge(records, members)
		}
	}
	for i := range records {
		if recurring[i] && records[i].DuplicateGroup == "" {
			records[i].Issues = append(records[i].Issues, recurringName)
		}
	}
}

// merge picks the most complete record in members as canonical, folds the
// others' data into it and points them at it via MergedInto.
func merge(records []model.Record, members []int) {
	canonical := members[0]
	best := completeness(&records[canonical])
	for _, i := range members[1:] {
		if s := completeness(&records[i]); s > best {
			canonical, best = i, s
		}
	}
	c := &records[canonical]
	groupID := c.ID
	for _, i := range members {
		if i == canonical {
			continue
		}
		r := &records[i]
		borrow(c, r)
		r.MergedInto = c.ID
		r.DuplicateGroup = groupID
		r.Issues = append(r.Issues, possibleDuplicate)
		label := r.Number
		if label == "" {
			label = r.Name
		}
		c.MergedFrom = append(c.MergedFrom, label)
	}
	c.DuplicateGroup = groupID
	c.Issues = append(c.Issues, possibleDuplicate)
}

// completeness scores how much a record has to offer as the canonical
// version of a duplicate group: more filled-in fields and an identifier
// that actually validates outrank a thinner or malformed row.
func completeness(r *model.Record) int {
	score := 0
	for _, v := range []string{r.Phone, r.Email, r.Website, r.Notes, r.Municipality, r.Status} {
		if v != "" {
			score++
		}
	}
	if validate.Number(r.Number) {
		score += 2
	}
	if r.Geometry != nil {
		score++
	}
	return score
}

// borrow fills any field canonical is missing with other's value, and
// folds in any source column canonical doesn't already carry. Name and
// Address are left untouched — the canonical keeps its own version of the
// text that made it the canonical.
func borrow(canonical, other *model.Record) {
	fields := []*string{
		&canonical.Number, &canonical.Enterprise, &canonical.Kind,
		&canonical.Phone, &canonical.Email, &canonical.Website,
		&canonical.Notes, &canonical.Municipality, &canonical.Status,
	}
	others := []string{
		other.Number, other.Enterprise, other.Kind,
		other.Phone, other.Email, other.Website,
		other.Notes, other.Municipality, other.Status,
	}
	for i, f := range fields {
		if *f == "" && others[i] != "" {
			*f = others[i]
		}
	}
	if canonical.Geometry == nil {
		canonical.Geometry = other.Geometry
	}
	if canonical.Source == nil {
		canonical.Source = map[string]any{}
	}
	for k, v := range other.Source {
		if _, ok := canonical.Source[k]; !ok {
			canonical.Source[k] = v
		}
	}
}
