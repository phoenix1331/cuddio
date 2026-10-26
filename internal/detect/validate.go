package detect

import "math"

// Luhn reports whether s passes the Luhn checksum used by payment card
// numbers. Spaces and hyphens between digits are ignored; any other
// character fails.
func Luhn(s string) bool {
	sum, digits := 0, 0
	double := false

	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c == ' ' || c == '-' {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}

		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		digits++
		double = !double
	}

	return digits > 1 && sum%10 == 0
}

// Entropy returns the Shannon entropy of s in bits per byte. Random tokens
// score high (around 4 to 6); words and repeated characters score low.
func Entropy(s string) float64 {
	if s == "" {
		return 0
	}

	var counts [256]int
	for i := range len(s) {
		counts[s[i]]++
	}

	n := float64(len(s))
	bits := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		bits -= p * math.Log2(p)
	}

	return bits
}

// MinEntropy returns a validator that accepts matches with at least the
// given entropy in bits per byte.
func MinEntropy(bits float64) func(string) bool {
	return func(match string) bool {
		return Entropy(match) >= bits
	}
}
