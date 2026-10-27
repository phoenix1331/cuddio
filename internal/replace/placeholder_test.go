package replace

import (
	"reflect"
	"strings"
	"testing"
)

func newPlaceholders(t *testing.T, format string) *Placeholders {
	t.Helper()

	p, err := NewPlaceholders(format)
	if err != nil {
		t.Fatalf("NewPlaceholders(%q) error = %v", format, err)
	}

	return p
}

// spans returns a match for every occurrence of each value in s.
func spans(s string, labels map[string]string) []Match {
	var matches []Match
	for value, label := range labels {
		for offset := 0; ; {
			i := strings.Index(s[offset:], value)
			if i < 0 {
				break
			}
			start := offset + i
			matches = append(matches, Match{Start: start, End: start + len(value), Label: label})
			offset = start + len(value)
		}
	}

	return matches
}

func TestTokenConsistency(t *testing.T) {
	t.Parallel()

	p := newPlaceholders(t, DefaultFormat)
	calls := []struct {
		label, value, want string
	}{
		{"EMAIL", "alice@example.com", "<EMAIL_1>"},
		{"EMAIL", "bob@example.com", "<EMAIL_2>"},
		{"IP", "10.0.0.1", "<IP_1>"},
		{"EMAIL", "alice@example.com", "<EMAIL_1>"},
		{"IP", "10.0.0.2", "<IP_2>"},
		// A value keeps its first token even when another rule matches it
		{"SECRET", "bob@example.com", "<EMAIL_2>"},
		{"EMAIL", "carol@example.com", "<EMAIL_3>"},
	}

	for _, c := range calls {
		if got := p.Token(c.label, c.value); got != c.want {
			t.Errorf("Token(%q, %q) = %q, want %q", c.label, c.value, got, c.want)
		}
	}

	want := []Pair{
		{"<EMAIL_1>", "alice@example.com"},
		{"<EMAIL_2>", "bob@example.com"},
		{"<IP_1>", "10.0.0.1"},
		{"<IP_2>", "10.0.0.2"},
		{"<EMAIL_3>", "carol@example.com"},
	}
	if got := p.Pairs(); !reflect.DeepEqual(got, want) {
		t.Errorf("Pairs() = %v, want %v", got, want)
	}
}

func TestPairsReturnsACopy(t *testing.T) {
	t.Parallel()

	p := newPlaceholders(t, DefaultFormat)
	p.Token("EMAIL", "alice@example.com")
	p.Pairs()[0].Value = "changed"

	if got := p.Pairs()[0].Value; got != "alice@example.com" {
		t.Errorf("Pairs()[0].Value = %q after changing a returned copy", got)
	}
}

func TestFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		want   string
	}{
		{DefaultFormat, "<EMAIL_1>"},
		{"[{label}-{n}]", "[EMAIL-1]"},
		{"{n}:{label}", "1:EMAIL"},
		{"__{label}{n}__", "__EMAIL1__"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			if got := newPlaceholders(t, tt.format).Token("EMAIL", "alice@example.com"); got != tt.want {
				t.Errorf("Token() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMustBeReversible(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"", "REDACTED", "<{label}>", "<{n}>", "<{LABEL}_{N}>"} {
		if _, err := NewPlaceholders(format); err == nil {
			t.Errorf("NewPlaceholders(%q) error = nil, want an error", format)
		}
	}
}

func TestReplaceConsistentAcrossLines(t *testing.T) {
	t.Parallel()

	labels := map[string]string{
		"alice@example.com": "EMAIL",
		"bob@example.com":   "EMAIL",
		"10.0.0.1":          "IP",
	}
	lines := []struct{ in, want string }{
		{"login bob@example.com from 10.0.0.1", "login <EMAIL_1> from <IP_1>"},
		{"alice@example.com emailed bob@example.com", "<EMAIL_2> emailed <EMAIL_1>"},
		{"retry from 10.0.0.1 for alice@example.com", "retry from <IP_1> for <EMAIL_2>"},
		{"nothing to hide here", "nothing to hide here"},
		{"", ""},
	}

	p := newPlaceholders(t, DefaultFormat)
	for _, line := range lines {
		if got := p.Replace(line.in, spans(line.in, labels)); got != line.want {
			t.Errorf("Replace(%q) = %q, want %q", line.in, got, line.want)
		}
	}
}

func TestReplaceNumbersInReadingOrder(t *testing.T) {
	t.Parallel()

	s := "a@example.com b@example.com c@example.com"
	// Candidates arrive in no particular order, as they do from several rules
	matches := []Match{
		{Start: 28, End: 41, Label: "EMAIL"},
		{Start: 0, End: 13, Label: "EMAIL"},
		{Start: 14, End: 27, Label: "EMAIL"},
	}

	want := "<EMAIL_1> <EMAIL_2> <EMAIL_3>"
	if got := newPlaceholders(t, DefaultFormat).Replace(s, matches); got != want {
		t.Errorf("Replace() = %q, want %q", got, want)
	}
}

func TestIsToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format, s string
		want      bool
	}{
		{DefaultFormat, "<EMAIL_1>", true},
		{DefaultFormat, "<GITLAB_PAT_12>", true},
		{DefaultFormat, "<1PASSWORD_SECRET_KEY_3>", true},
		{DefaultFormat, "<EMAIL_0>", false},
		{DefaultFormat, "<email_1>", false},
		{DefaultFormat, "x <EMAIL_1>", false},
		{DefaultFormat, "<EMAIL_1>x", false},
		{DefaultFormat, "", false},
		{"[[{label}.{n}]]", "[[EMAIL.2]]", true},
		{"[[{label}.{n}]]", "[[EMAIL_2]]", false},
		{"{n}-{label}", "7-IP", true},
	}
	for _, tt := range tests {
		if got := newPlaceholders(t, tt.format).IsToken(tt.s); got != tt.want {
			t.Errorf("format %q: IsToken(%q) = %v, want %v", tt.format, tt.s, got, tt.want)
		}
	}
}
