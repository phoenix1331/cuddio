// Package names finds the known name closest to a mistyped one, so an error
// can suggest a fix. Only names within a small edit distance of a known name
// are ever repeated: anything further away may be a value typed in the wrong
// place, and a value may be a secret.
package names

// maxDistance is the most edits a typo may be from a known name.
const maxDistance = 2

// Nearest returns the first of known within two edits of name, preferring
// fewer edits, and whether there was one.
func Nearest(name string, known []string) (string, bool) {
	best, bestDistance := "", maxDistance+1
	for _, k := range known {
		if d := distance(name, k); d < bestDistance {
			best, bestDistance = k, d
		}
	}

	return best, bestDistance <= maxDistance
}

// distance is the Levenshtein distance between a and b in bytes.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}
