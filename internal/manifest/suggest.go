package manifest

// Suggest returns the best did-you-mean candidates for a misspelled field,
// using Levenshtein distance with a cap of `maxDistance`. If no candidate is
// within the cap, returns nil. If multiple candidates tie for best distance,
// all are returned in `candidates` order.
func Suggest(input string, candidates []string, maxDistance int) []string {
	if input == "" || maxDistance < 0 {
		return nil
	}
	best := maxDistance + 1
	var matches []string
	for _, c := range candidates {
		d := levenshtein(input, c)
		if d > maxDistance {
			continue
		}
		if d < best {
			best = d
			matches = matches[:0]
			matches = append(matches, c)
		} else if d == best {
			matches = append(matches, c)
		}
	}
	return matches
}

// levenshtein computes the standard edit distance between two strings.
// Two-row implementation, no allocations beyond the row buffers.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
