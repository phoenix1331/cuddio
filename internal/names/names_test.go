package names

import "testing"

func TestNearest(t *testing.T) {
	t.Parallel()

	known := []string{"email", "jwt", "pii", "secret"}

	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"exact", "email", "email", true},
		{"one transposition", "emial", "email", true},
		{"one extra letter", "secrets", "secret", true},
		{"two edits", "emaill2", "email", true},
		{"three edits", "emaill23", "", false},
		{"short typo", "pi", "pii", true},
		{"prefers fewer edits", "jwts", "jwt", true},
		{"far away", "sk_live_51Hc7abcdef", "", false},
		{"lowercase hex key", "4f1c2b9e8d7a6f5e4d3c2b1a0f9e8d7c", "", false},
		{"empty", "", "jwt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := Nearest(tt.in, known)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("Nearest(%q) = %q, %v, want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestDistance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"email", "emial", 2},
		{"secret", "secrets", 1},
	}

	for _, tt := range tests {
		if got := distance(tt.a, tt.b); got != tt.want {
			t.Errorf("distance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
