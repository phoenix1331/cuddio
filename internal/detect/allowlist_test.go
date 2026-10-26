package detect

import (
	"strings"
	"testing"
)

func TestBuiltinAllowlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"reserved domain", "example.com", true},
		{"invalid TLD domain", "example.invalid", true},
		{"test address", "test@example.com", true},
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv6 loopback", "::1", true},
		{"AWS documentation access key", "AKIAIOSFODNN7EXAMPLE", true},
		{"AWS documentation secret key", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", true},
		{"Stripe documentation secret key", "sk_test_4eC39HqLyjWDarjtT1zdp7dc", true},

		// Near misses are still redacted
		{"other address at a reserved domain", "alice@example.com", false},
		{"test address at a subdomain", "test@mail.example.com", false},
		{"different case", "Test@Example.com", false},
		{"lookalike domain", "example.com.attacker.io", false},
		{"similar TLD", "alice@example.community", false},
		{"other loopback-like address", "127.0.0.10", false},
		{"private address", "10.0.0.1", false},
		{"other IPv6 address", "::10", false},
		{"AWS key one character off", "AKIAIOSFODNN7EXAMPLF", false},
		{"other Stripe test key", "sk_test_4eC39HqLyjWDarjtT1zdp7dX", false},
		{"surrounding whitespace", " 127.0.0.1", false},
		{"empty", "", false},
	}

	allow, err := NewAllowlist(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := allow.Allows(tt.value); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestConfigAllowlist(t *testing.T) {
	t.Parallel()

	allow, err := NewAllowlist(
		[]string{"ops@example.com", "sk_live_shared_demo"},
		[]string{`^dev-[0-9]+$`, `@staging\.example\.com$`},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"exact value", "ops@example.com", true},
		{"second exact value", "sk_live_shared_demo", true},
		{"anchored pattern", "dev-42", true},
		{"suffix pattern", "alice@staging.example.com", true},
		{"built-ins still apply", "127.0.0.1", true},

		{"exact value with a suffix", "ops@example.com.au", false},
		{"anchored pattern with extra text", "dev-42-prod", false},
		{"anchored pattern with letters", "dev-abc", false},
		{"suffix pattern on another domain", "alice@staging.example.co", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := allow.Allows(tt.value); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestAllowlistPatternErrorsHideThePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{"unclosed group", `(sk_live_DO_NOT_ECHO`, "allowlist pattern 2: invalid pattern: missing closing )"},
		{"backreference", `(sk_live_DO_NOT_ECHO)\1`, "allowlist pattern 2: invalid pattern: invalid escape sequence"},
		{"lookahead", `sk_live_DO_NOT_ECHO(?=x)`, "allowlist pattern 2: invalid pattern: invalid or unsupported Perl syntax"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewAllowlist(nil, []string{"fine", tt.pattern})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("NewAllowlist() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "sk_live") {
				t.Errorf("error repeats the pattern: %q", err)
			}
		})
	}
}
