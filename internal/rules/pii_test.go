package rules

import (
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
)

// piiCases lists, per rule ID, matches, near misses and allowlisted values.
// Phone numbers come from Ofcom's drama ranges, addresses from the
// documentation ranges, and card numbers are published test cards.
var piiCases = map[string][]ruleCase{
	"email": {
		{"plain", "from alice@shop.test today", "from <EMAIL_1> today"},
		{"plus and dots", "a.b+tag@mail.shop.co.uk", "<EMAIL_1>"},
		{"mailto", "mailto:alice@shop.test", "mailto:<EMAIL_1>"},
		{"url-encoded", "/reset?email=alice%40shop.test&x=1", "/reset?email=<EMAIL_1>&x=1"},
		{"in json", `{"email":"bob@example.invalid"}`, `{"email":"<EMAIL_1>"}`},
		{"retina image", "logo@2x.png", ""},
		{"ssh remote", "git@github.com:phoenix1331/cuddio.git", ""},
		{"no dot in domain", "root@localhost", ""},
		{"handle", "@alice", ""},
		{"credentials before a host", "app:hunter@db.shop.test", ""},
		{"test address allowlisted", "test@example.com", ""},
	},
	"uk-phone": {
		{"mobile", "call 07700 900123 now", "call <PHONE_1> now"},
		{"international", "+44 7700 900123", "<PHONE_1>"},
		{"international with (0)", "+44 (0)20 7946 0018", "<PHONE_1>"},
		{"london in brackets", "(020) 7946 0018", "<PHONE_1>"},
		{"hyphens", "0161-496-0000", "<PHONE_1>"},
		{"no spaces", "01632960000", "<PHONE_1>"},
		{"too long", "077009001234567", ""},
		{"too short", "0770 0900", ""},
		{"date", "2026-10-26 10:15:02", ""},
	},
	"e164-phone": {
		{"compact", "tel +14155550100", "tel <PHONE_1>"},
		{"spaced", "+1 415 555 0100", "<PHONE_1>"},
		{"france", "+33 1 23 45 67 89", "<PHONE_1>"},
		{"too short", "+123456", ""},
		{"after a word character", "a+14155550100", ""},
		{"leading zero", "+0123456789", ""},
	},
	"ip-address": {
		{"ipv4", "client 203.0.113.9 connected", "client <IP_1> connected"},
		{"adjacent addresses", "203.0.113.9 203.0.113.10,198.51.100.1", "<IP_1> <IP_2>,<IP_3>"},
		{"adjacent ipv6", "2001:db8::1 2001:db8::2", "<IP_1> <IP_2>"},
		{"in a url", "http://198.51.100.7/health", "http://<IP_1>/health"},
		{"nginx log", "203.0.113.9 - - [26/Oct/2026]", "<IP_1> - - [26/Oct/2026]"},
		{"ipv6", "from 2001:db8::1 ok", "from <IP_1> ok"},
		{"ipv6 with port", "[2001:db8:85a3::8a2e:370:7334]:443", "[<IP_1>]:443"},
		{"private by default", "10.0.0.5", ""},
		{"loopback allowlisted", "127.0.0.1 and ::1", ""},
		{"user agent version", "Chrome/130.0.0.0 Safari/537.36", ""},
		{"five parts: first four redacted, as RE2 has no lookahead", "1.2.3.4.5", "<IP_1>.5"},
		{"octet out of range", "999.1.1.1", ""},
		{"too many digits", "1.2.3.4567", ""},
		{"time", "10:15:02", ""},
		{"php static call", "Feed::add() and Dead::beef", ""},
		{"mac address", "aa:bb:cc:dd:ee:01", ""},
	},
	"private-ip-address": {
		{"rfc 1918", "db at 10.0.0.5", "db at <IP_1>"},
		{"home network", "192.168.1.20", "<IP_1>"},
		{"link-local ipv6", "fe80::1", "<IP_1>"},
		{"cgnat", "100.64.0.1", "<IP_1>"},
		{"unspecified", "listen 0.0.0.0", ""},
		{"loopback", "127.0.0.2", ""},
	},
	"payment-card": {
		{"visa spaced", "card 4111 1111 1111 1111 ok", "card <CARD_1> ok"},
		{"mastercard", "5555555555554444", "<CARD_1>"},
		{"amex hyphens", "3782-822463-10005", "<CARD_1>"},
		{"luhn fails", "4111111111111112", ""},
		{"millisecond timestamp", "1729936502123", ""},
		{"unknown prefix", "1234567812345670", ""},
		{"too short", "411111111111", ""},
	},
	"uk-ni-number": {
		{"compact", "NI AB123456C", "NI <NI_NUMBER_1>"},
		{"spaced", "AB 12 34 56 C", "<NI_NUMBER_1>"},
		{"HMRC example prefix", "QQ123456C", ""},
		{"never issued prefix", "GB123456A", ""},
		{"bad suffix", "AB123456E", ""},
		{"no suffix", "AB123456", ""},
		{"lowercase", "ab123456c", ""},
	},
	"uk-postcode": {
		{"full", "London SW1A 1AA", "London <POSTCODE_1>"},
		{"short outward code", "M1 1AE", "<POSTCODE_1>"},
		{"no space", "CR26XH", "<POSTCODE_1>"},
		{"letter C in unit", "SW1A 1CA", ""},
		{"Q first", "Q1 1AA", ""},
	},
	"home-directory": {
		{"linux", "/home/darren/app/vendor", "/home/<USER_1>/app/vendor"},
		{"macos", "/Users/alice/Library", "/Users/<USER_1>/Library"},
		{"windows with a space", `C:\Users\Jane Smith\AppData`, `C:\Users\<USER_1>\AppData`},
		{"windows in json", `"C:\\Users\\bob\\app"`, `"C:\\Users\\<USER_1>\\app"`},
		{"windows forward slashes", "c:/users/bob/app", "c:/users/<USER_1>/app"},
		{"end of path", "cd /home/forge", "cd /home/<USER_1>"},
		{"shared profile", "/Users/Shared/data", ""},
		{"reference", "/home/${USER}/app", ""},
		{"similar directory", "/var/homepage/x", ""},
	},
}

func TestOptionalRulesAreOffByDefault(t *testing.T) {
	t.Parallel()

	for _, rule := range Builtin() {
		if !rule.Optional {
			continue
		}
		for _, tt := range piiCases[rule.ID] {
			if tt.want != "" && clean(t, tt.in) != tt.in {
				t.Errorf("%s/%s: redacted without enabling the rule", rule.ID, tt.name)
			}
		}
	}
}

func TestOptionalRules(t *testing.T) {
	t.Parallel()

	var optional []string
	for _, rule := range Builtin() {
		if rule.Optional {
			optional = append(optional, rule.ID)
		}
	}
	if got, want := strings.Join(optional, ","), "private-ip-address,uk-postcode"; got != want {
		t.Errorf("optional rules = %s, want %s", got, want)
	}
}

func TestPIIOverlaps(t *testing.T) {
	t.Parallel()

	tests := []ruleCase{
		{"email in a dsn host is not an email", "mysql://app:hunter2@db.shop.test/x", "mysql://app:<PASSWORD_1>@db.shop.test/x"},
		{"same email, same token", "alice@shop.test, ALICE: alice@shop.test", "<EMAIL_1>, ALICE: <EMAIL_1>"},
		{"secret wins a tie with PII", "DB_PASSWORD=alice@shop.test", "DB_PASSWORD=<SECRET_1>"},
		{"username in a path, then elsewhere", "/home/forge/app ran as forge", "/home/<USER_1>/app ran as forge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := clean(t, tt.in); got != tt.want {
				t.Errorf("clean(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuiltinCategories(t *testing.T) {
	t.Parallel()

	for _, rule := range secrets() {
		if rule.Category != detect.Secret {
			t.Errorf("%s: category %s, want secret", rule.ID, rule.Category)
		}
	}
	for _, rule := range pii() {
		if rule.Category != detect.PII {
			t.Errorf("%s: category %s, want pii", rule.ID, rule.Category)
		}
	}
}
