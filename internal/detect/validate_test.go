package detect

import (
	"math"
	"testing"
)

func TestLuhn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"visa test card", "4242424242424242", true},
		{"mastercard test card", "5555555555554444", true},
		{"amex test card", "378282246310005", true},
		{"spaces between groups", "4242 4242 4242 4242", true},
		{"hyphens between groups", "4242-4242-4242-4242", true},
		{"one digit off", "4242424242424241", false},
		{"two digits swapped", "4242424242424224", false},
		{"letter inside", "42424242424242a2", false},
		{"dot inside", "4242.4242.4242.4242", false},
		{"single digit", "0", false},
		{"empty", "", false},
		{"only separators", " - ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Luhn(tt.in); got != tt.want {
				t.Errorf("Luhn(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEntropy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"empty", "", 0},
		{"one repeated character", "aaaaaaaa", 0},
		{"two characters evenly", "abababab", 1},
		{"sixteen distinct characters", "0123456789abcdef", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Entropy(tt.in); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("Entropy(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMinEntropy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"random-looking token", "q8Zr2LxP0vNk7TbW", true},
		{"exactly at the threshold", "0123456789abcdef", true},
		{"placeholder password", "password", false},
		{"repeated characters", "xxxxxxxxxxxxxxxx", false},
	}

	high := MinEntropy(4)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := high(tt.in); got != tt.want {
				t.Errorf("MinEntropy(4)(%q) = %v (entropy %.2f), want %v", tt.in, got, Entropy(tt.in), tt.want)
			}
		})
	}
}
