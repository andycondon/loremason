// Package suggest finds the name someone probably meant.
package suggest

// Closest returns the candidate nearest to s by edit distance, or "" when
// none is within max edits. Ties go to the earlier candidate.
func Closest(s string, candidates []string, max int) string {
	best, bestD := "", max+1
	for _, c := range candidates {
		if d := distance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// Near is Closest with a limit suited to words: one edit, plus one for
// every four characters.
func Near(s string, candidates []string) string {
	return Closest(s, candidates, 1+len(s)/4)
}

// distance is the Levenshtein distance, counting a swap of neighbours as one.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// Hint ends a message with a suggestion, or with nothing when there is none.
func Hint(s string) string {
	if s == "" {
		return ""
	}
	return "; did you mean `" + s + "`?"
}
