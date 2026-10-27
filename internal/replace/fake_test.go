package replace

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFakeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		label, value string
		n            int
		want         string
	}{
		{"EMAIL", "alice@shop.test", 1, "user1@example.invalid"},
		{"BILLING_EMAIL", "x", 12, "user12@example.invalid"},
		{"PHONE", "07700 900123", 1, "07700 900001"},
		{"PHONE", "+44 7700 900456", 2, "+44 7700 900002"},
		{"PHONE", "0044 (7700) 900-456", 3, "0044 (7700) 900-003"},
		{"PHONE", "+44 77009", 4, "+447700900004"},
		{"MOBILE", "7700", 5, "07700900005"},
		{"PHONE", "07700900123", 1000, "01632960000"},
		{"PHONE", "07700900123", 15000, "07700900000"},
		{"IP", "203.0.113.9", 1, "192.0.2.1"},
		{"IP_ADDRESS", "10.0.0.1", 255, "198.51.100.1"},
		{"IP", "10.0.0.1", 763, "192.0.2.1"},
		{"IP", "2001:db8::7", 26, "2001:db8::1a"},
		{"CARD", "4242 4242 4242 4242", 1, "4242 4242 4242 4242"},
		{"CARD", "5555-5555-5555-4444", 2, "4000-0566-5566-5556"},
		{"CARD", "378282246310005", 2, "371449635398431"},
		{"CARD", "36227206271667", 1, "36227206271667"},
		{"CARD", "4242 42", 3, "5555555555554444"},
		{"NI_NUMBER", "AB 12 34 56 C", 7, "QQ 00 00 07 C"},
		{"NI_NUMBER", "AB123", 7, "QQ000007C"},
		{"POSTCODE", "CF10 1AA", 1, "ZZ99 1AA"},
		{"SHIPPING_POSTCODE", "SW1A1AA", 123, "ZZ993AM"},
		{"PHONE", "0044 77", 6, "00447700900006"},
		{"POSTCODE", "M1 1AA", 1, "ZZ99 1AA"},
		{"DOB", "1990-05-01", 1, "1950-04-08"},
		{"DOB", "1990-05-01 00:00:00", 1, "1950-04-08 00:00:00"},
		{"DATE_OF_BIRTH", "01/05/1990", 1, "08/04/1950"},
		{"DOB", "May 1990", 1, "1950-04-08"},
		{"ADDRESS", "12 Heol y Frenhines\nCaerdydd", 3, "3 Example Street, Testville"},
		{"NAME", "Alice O'Brien", 1, "Alex Smith"},
		{"BILLING_NAME", "Bob Jones", 2, "Sam Davies"},
		{"NAME", "Bob Jones", 22, "Sam Evans"},
		{"NAME", "cli", 2, "Sam"},
		{"USER", "jane", 4, "user4"},
		{"PASSWORD", "$2y$12$abc", 1, "REDACTED"},
		{"SMTP_PASSWORD", "hunter2", 9, "REDACTED"},
		{"PRIVATE_KEY", "-----BEGIN KEY-----", 1, "REDACTED"},
		{"AWS_KEY", "AKIA_1", 1, scramble("AWS_KEY", "AKIA_1", 1)},
	}
	for _, tt := range tests {
		if got := fakeFor(tt.label, tt.value, tt.n); got != tt.want {
			t.Errorf("fakeFor(%q, %q, %d) = %q, want %q", tt.label, tt.value, tt.n, got, tt.want)
		}
	}
}

func TestScramble(t *testing.T) {
	t.Parallel()

	value := "sk_live_Ab3Cd5Ef7Gh9-xyz.Llŷr"
	got := scramble("STRIPE_KEY", value, 1)

	if got == value || utf8.RuneCountInString(got) != utf8.RuneCountInString(value) {
		t.Fatalf("scramble() = %q, want a different value of the same length", got)
	}
	if got != scramble("STRIPE_KEY", value, 1) {
		t.Error("scramble() is not deterministic")
	}
	if scramble("STRIPE_KEY", value, 2) == got || scramble("TOKEN", value, 1) == got {
		t.Error("scramble() gives the same fake for a different counter or label")
	}
	if !strings.ContainsAny(got, "_-.") || strings.Count(got, "_") != 2 {
		t.Errorf("scramble() = %q, want punctuation kept", got)
	}
	for i, r := range []rune(got) {
		orig := []rune(value)[i]
		if (orig >= 'A' && orig <= 'Z') != (r >= 'A' && r <= 'Z') || (orig >= '0' && orig <= '9') != (r >= '0' && r <= '9') {
			t.Errorf("scramble() changed the class of %q at %d to %q", orig, i, r)
		}
		if r >= 0x80 {
			t.Errorf("scramble() kept non-ASCII %q", r)
		}
	}
	if scramble("X", "", 1) != "" {
		t.Error("scramble() of an empty value is not empty")
	}
}

func TestScrambleNeverReturnsTheValue(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"a", "Z", "z", "9", "0", "_", "ŷ"} {
		for n := 1; n <= 200; n++ {
			if got := scramble("SECRET", value, n); got == value {
				t.Fatalf("scramble(%q, %d) returned the value", value, n)
			}
		}
	}
}

func TestRotate(t *testing.T) {
	t.Parallel()

	for in, want := range map[rune]rune{'a': 'b', 'z': 'a', 'Z': 'A', '9': '0', '4': '5', '-': 'x'} {
		if got := rotate(in); got != want {
			t.Errorf("rotate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFakesConsistency(t *testing.T) {
	t.Parallel()

	f := NewFakes()
	a := f.Token("EMAIL", "alice@shop.test")
	b := f.Token("EMAIL", "bob@shop.test")
	if a != "user1@example.invalid" || b != "user2@example.invalid" {
		t.Errorf("tokens = %q, %q", a, b)
	}
	if got := f.Token("BILLING_EMAIL", "alice@shop.test"); got != a {
		t.Errorf("same value under another label = %q, want %q", got, a)
	}
	if got := f.Token("PHONE", "07700 900123"); got != "07700 900001" {
		t.Errorf("each label counts from 1: got %q", got)
	}
	if f.IsToken(a) {
		t.Error("IsToken() = true for a fake")
	}
}

func TestFakesFit(t *testing.T) {
	t.Parallel()

	f := NewFakes()
	tests := []struct {
		fake string
		max  int
		want string
	}{
		{"user12@example.invalid", 0, "user12@example.invalid"},
		{"user12@example.invalid", 30, "user12@example.invalid"},
		{"user12@example.invalid", 6, "user12"},
		{"4242", 4, "4242"},
		{"Llŷr", 3, "Llŷ"},
	}
	for _, tt := range tests {
		if got := f.Fit(tt.fake, tt.max); got != tt.want {
			t.Errorf("Fit(%q, %d) = %q, want %q", tt.fake, tt.max, got, tt.want)
		}
	}

	p := newPlaceholders(t, DefaultFormat)
	if got := p.Fit("<EMAIL_1>", 3); got != "<EMAIL_1>" {
		t.Errorf("Placeholders.Fit() = %q, want the token whole", got)
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	f := NewFakes()
	s := "mail alice@shop.test or bob@shop.test"
	got := Apply(s, spans(s, map[string]string{"alice@shop.test": "EMAIL", "bob@shop.test": "EMAIL"}), func(m Match, value string) string { return f.Token(m.Label, value) })
	if want := "mail user1@example.invalid or user2@example.invalid"; got != want {
		t.Errorf("Apply() = %q, want %q", got, want)
	}
}

func TestFakesNeverReturnTheValue(t *testing.T) {
	t.Parallel()

	f := NewFakes()
	if got := f.Token("CARD", "4242 4242 4242 4242"); got != "4000 0566 5566 5556" {
		t.Errorf("a test card's fake = %q, want the next test card", got)
	}
	if got := f.Token("EMAIL", "user1@example.invalid"); got != "user2@example.invalid" {
		t.Errorf("a fake fed back in = %q, want the next fake", got)
	}
	if got := f.Token("PASSWORD", "REDACTED"); got != "REDACTED" {
		t.Errorf("a fixed fake = %q, want REDACTED", got)
	}
}

func TestFakeNamesAreDistinct(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for n := 1; n <= len(firstNames)*len(lastNames); n++ {
		name := fakeName("A B", n)
		if seen[name] {
			t.Fatalf("fakeName(%d) = %q, already used", n, name)
		}
		seen[name] = true
	}
}
