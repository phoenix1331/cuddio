package rules

import (
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
)

// fake returns n deterministic characters that look random: mixed case,
// digits and 5 bits per byte of entropy. Fixtures are assembled from it at
// test time so no live-format token is ever committed.
func fake(n int) string {
	const chars = "Ab3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1Wx"

	var b strings.Builder
	for i := range n {
		b.WriteByte(chars[i%len(chars)])
	}

	return b.String()
}

// clean runs s through every built-in rule that is on by default plus any in
// enable, the built-in allowlist and the default placeholder format, as
// cuddio does with no config.
func clean(t *testing.T, s string, enable ...string) string {
	t.Helper()

	reg, err := detect.NewRegistry(Builtin())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := reg.Select(enable, nil)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := detect.NewAllowlist(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		t.Fatal(err)
	}

	return p.Replace(s, detect.NewDetector(selected, allow).Find(s))
}

type ruleCase struct {
	name string
	in   string
	// want is the cleaned output; empty means the input must pass unchanged.
	want string
}

// secretCases lists, per rule ID, matches, near misses and allowlisted
// values. Near misses and allowlisted values have an empty want.
var secretCases = map[string][]ruleCase{
	"aws-access-key": {
		{"access key", "id AKIA" + strings.ToUpper(fake(16)) + " used", "id <AWS_KEY_1> used"},
		{"session key", "ASIA" + strings.ToUpper(fake(16)), "<AWS_KEY_1>"},
		{"too short", "AKIA" + strings.ToUpper(fake(15)), ""},
		{"lowercase", "akia" + strings.ToLower(fake(16)), ""},
		{"documentation key allowlisted", "AKIAIOSFODNN7EXAMPLE", ""},
	},
	"aws-secret-key": {
		{"env style", "AWS_SECRET_ACCESS_KEY=" + fake(40), "AWS_SECRET_ACCESS_KEY=<AWS_SECRET_1>"},
		{"ini style", "aws_secret_access_key = " + fake(40) + "\n", "aws_secret_access_key = <AWS_SECRET_1>\n"},
		{"json", `"awsSecretKey": "` + fake(40) + `"`, `"awsSecretKey": "<AWS_SECRET_1>"`},
		{"41 characters", "aws_secret_key: " + strings.Repeat("a", 41), ""},
		{"reference", "aws_secret_access_key = ${AWS_SECRET}", ""},
		{"documentation key allowlisted", "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", ""},
	},
	"github-token": {
		{"classic", "token ghp_" + fake(36), "token <GITHUB_TOKEN_1>"},
		{"oauth", "gho_" + fake(36), "<GITHUB_TOKEN_1>"},
		{"fine grained", "github_pat_" + fake(22) + "_" + fake(59), "<GITHUB_TOKEN_1>"},
		{"too short", "ghp_" + fake(35), ""},
		{"unknown prefix", "ghx_" + fake(36), ""},
	},
	"stripe-key": {
		{"live secret", "sk_live_" + fake(24), "<STRIPE_KEY_1>"},
		{"restricted", "rk_test_" + fake(24), "<STRIPE_KEY_1>"},
		{"webhook secret", "whsec_" + fake(32), "<STRIPE_KEY_1>"},
		{"publishable key is public", "pk_live_" + fake(24), ""},
		{"too short", "sk_live_" + fake(9), ""},
		{"documentation key allowlisted", "sk_test_4eC39HqLyjWDarjtT1zdp7dc", ""},
	},
	"slack-token": {
		{"bot", "xoxb-1234567890-" + fake(24), "<SLACK_TOKEN_1>"},
		{"app", "xapp-1-" + fake(30), "<SLACK_TOKEN_1>"},
		{"unknown type", "xoxz-1234567890-" + fake(24), ""},
		{"too short", "xoxb-12345", ""},
	},
	"slack-webhook": {
		{"service", "https://hooks.slack.com/services/T0" + strings.ToUpper(fake(8)) + "/B0" + strings.ToUpper(fake(8)) + "/" + fake(24), "<SLACK_WEBHOOK_1>"},
		{"other host", "https://hooks.example.com/services/T00/B00/" + fake(24), ""},
		{"no path", "https://hooks.slack.com/services/", ""},
	},
	"openai-key": {
		{"project", "sk-proj-" + fake(48), "<OPENAI_KEY_1>"},
		{"legacy", "sk-" + fake(20) + "T3BlbkFJ" + fake(20), "<OPENAI_KEY_1>"},
		{"bare sk- prefix", "sk-" + fake(48), ""},
		{"too short", "sk-proj-" + fake(19), ""},
	},
	"anthropic-key": {
		{"api key", "sk-ant-api03-" + fake(90), "<ANTHROPIC_KEY_1>"},
		{"admin key", "sk-ant-admin01-" + fake(90), "<ANTHROPIC_KEY_1>"},
		{"unknown kind", "sk-ant-foo03-" + fake(90), ""},
		{"too short", "sk-ant-api03-" + fake(19), ""},
	},
	"google-api-key": {
		{"api key", "key AIza" + fake(35), "key <GOOGLE_KEY_1>"},
		{"oauth client secret", "GOCSPX-" + fake(28), "<GOOGLE_KEY_1>"},
		{"too short", "AIza" + fake(34), ""},
	},
	"jwt": {
		{"token", "jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0." + fake(43), "jwt <JWT_1>"},
		{"unsigned", "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.", "<JWT_1>"},
		{"payload not JSON", "eyJhbGciOiJIUzI1NiJ9.abcdefgh." + fake(43), ""},
	},
	"authorization-header": {
		{"bearer", "Authorization: Bearer " + fake(20) + "\r", "Authorization: Bearer <AUTH_1>\r"},
		{"basic", "authorization: Basic dXNlcjpwYXNz", "authorization: Basic <AUTH_1>"},
		{"json", `{"authorization":"Token ` + fake(20) + `","x":1}`, `{"authorization":"Token <AUTH_1>","x":1}`},
		{"no scheme", "Proxy-Authorization: " + fake(12), "Proxy-Authorization: <AUTH_1>"},
		{"word in prose", "authorization failed for user", ""},
		{"reference", "Authorization: ${TOKEN}", ""},
	},
	"bearer-token": {
		{"in a log line", "sent bearer " + fake(24) + " to api", "sent bearer <TOKEN_1> to api"},
		{"too short", "Bearer abc", ""},
		{"word in prose", "the bearer of bad news", ""},
	},
	"cookie": {
		{"request header", "Cookie: laravel_session=" + fake(40) + "; XSRF-TOKEN=abc", "Cookie: <COOKIE_1>"},
		{"response header", "Set-Cookie: sid=" + fake(16) + "; Path=/; HttpOnly", "Set-Cookie: <COOKIE_1>"},
		{"json", `"cookie":"a=b","ip":"x"`, `"cookie":"<COOKIE_1>","ip":"x"`},
		{"different key", "cookie_consent=1", ""},
		{"empty", "Cookie: ", ""},
	},
	"url-password": {
		{"mysql dsn", "mysql://app:hunter2@db.internal:3306/shop", "mysql://app:<PASSWORD_1>@db.internal:3306/shop"},
		{"redis", "redis://default:s3cr3t-Pa55@cache:6379", "redis://default:<PASSWORD_1>@cache:6379"},
		{"no password", "postgres://app@db/shop", ""},
		{"port, not password", "https://example.org:8443/path", ""},
		{"reference", "mysql://app:${DB_PASSWORD}@db/shop", ""},
	},
	"token-query-param": {
		{"token", "GET /reset?token=" + fake(20) + "&lang=en", "GET /reset?token=<TOKEN_1>&lang=en"},
		{"signed url", "/file.pdf?expires=1&signature=" + fake(30), "/file.pdf?expires=1&signature=<TOKEN_1>"},
		{"api key", "?api_key=abc123 HTTP/1.1", "?api_key=<TOKEN_1> HTTP/1.1"},
		{"harmless param", "/search?q=token&page=2", ""},
		{"name only ends in token", "/search?mytoken=abc", ""},
	},
	"secret-assignment": {
		{"bare", "DB_PASSWORD=hunter2", "DB_PASSWORD=<SECRET_1>"},
		{"double quoted with spaces", `MAIL_PASSWORD="correct horse battery"`, `MAIL_PASSWORD="<SECRET_1>"`},
		{"single quoted", "API_TOKEN='abc'", "API_TOKEN='<SECRET_1>'"},
		{"inline comment kept", "STRIPE_SECRET=abc # rotate monthly", "STRIPE_SECRET=<SECRET_1> # rotate monthly"},
		{"export", "export AWS_SESSION_TOKEN=abc", "export AWS_SESSION_TOKEN=<SECRET_1>"},
		{"commented out", "# OLD_API_KEY=abc", "# OLD_API_KEY=<SECRET_1>"},
		{"indented", "  PUSHER_APP_KEY=abc", "  PUSHER_APP_KEY=<SECRET_1>"},
		{"no secret in name", "APP_NAME=Laravel", ""},
		{"empty", "DB_PASSWORD=", ""},
		{"empty quotes", `DB_PASSWORD=""`, ""},
		{"null", "DB_PASSWORD=null", ""},
		{"reference", "REDIS_PASSWORD=${DB_PASSWORD}", ""},
		{"already cleaned", "DB_PASSWORD=<SECRET_1>", ""},
		{"not at line start", "echo DB_PASSWORD=abc", ""},
	},
	"secret-field": {
		{"json", `{"email":"a","password":"hunter2"}`, `{"email":"a","password":"<SECRET_1>"}`},
		{"escaped quote", `{"api_key":"ab\"cd"}`, `{"api_key":"<SECRET_1>"}`},
		{"php array", `'client_secret' => 'abc'`, `'client_secret' => '<SECRET_1>'`},
		{"number value", `{"token_count": 12}`, ""},
		{"empty", `{"password":""}`, ""},
		{"no secret in name", `{"username":"alice"}`, ""},
	},
	"laravel-app-key": {
		{"env", "APP_KEY=base64:" + fake(43) + "=", "APP_KEY=<APP_KEY_1>"},
		{"in a config dump", "'key' => 'base64:" + fake(43) + "='", "'key' => '<APP_KEY_1>'"},
		{"too short", "x base64:" + strings.Repeat("A", 42) + "=", ""},
	},
	"private-key": {
		{
			"whole block",
			"key:\n-----BEGIN RSA PRIVATE KEY-----\n" + fake(64) + "\n" + fake(20) + "\n-----END RSA PRIVATE KEY-----\ndone",
			"key:\n<PRIVATE_KEY_1>\ndone",
		},
		{"unterminated", "-----BEGIN OPENSSH PRIVATE KEY-----\r\n" + fake(64) + "\r\n", "<PRIVATE_KEY_1>"},
		{"escaped in json", `"-----BEGIN PRIVATE KEY-----\n` + fake(64) + `\n-----END PRIVATE KEY-----"`, `"<PRIVATE_KEY_1>"`},
		{"public key", "-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----", ""},
		{"certificate", "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----", ""},
	},
	"high-entropy-assignment": {
		{"yaml", "client_id: " + fake(40), "client_id: <SECRET_1>"},
		{"json", `{"signingValue":"` + fake(36) + `"}`, `{"signingValue":"<SECRET_1>"}`},
		{"git hash", "commit=" + strings.Repeat("9f86d081884c7d65", 3), ""},
		{"uuid", "id: 123e4567-e89b-12d3-a456-426614174000", ""},
		{"path", "path=/var/www/html/storage/framework/cache/data", ""},
		{"url", "see https://hooks.example.com/services/T00/B00/" + fake(24), ""},
		{"too short", "client_id: " + fake(31), ""},
		{"no digits", "class: AbCdEfGhIjKlMnOpQrStUvWxYzAbCdEfGhIj", ""},
	},
}

// allCases merges the per-rule cases of every category.
func allCases() map[string][]ruleCase {
	all := map[string][]ruleCase{}
	for _, cases := range []map[string][]ruleCase{secretCases, piiCases} {
		for id, c := range cases {
			all[id] = append(all[id], c...)
		}
	}

	return all
}

// TestRules runs every case through all default rules plus the case's own
// rule, so optional rules are tested too and overlaps show up.
func TestRules(t *testing.T) {
	t.Parallel()

	for id, cases := range allCases() {
		for _, tt := range cases {
			t.Run(id+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				want := tt.want
				if want == "" {
					want = tt.in
				}
				if got := clean(t, tt.in, id); got != want {
					t.Errorf("clean(%q)\n got %q\nwant %q", tt.in, got, want)
				}
			})
		}
	}
}

// TestRulesFireOnTheirOwn runs each positive case through its rule alone, so
// a case cannot pass only because a different rule caught it.
func TestRulesFireOnTheirOwn(t *testing.T) {
	t.Parallel()

	cases := allCases()
	for _, rule := range Builtin() {
		det := detect.NewDetector([]detect.Rule{rule}, nil)
		for _, tt := range cases[rule.ID] {
			if tt.want != "" && len(det.Find(tt.in)) == 0 {
				t.Errorf("%s/%s: rule does not match on its own", rule.ID, tt.name)
			}
		}
	}
}

func TestEveryRuleHasCases(t *testing.T) {
	t.Parallel()

	cases := allCases()
	for _, rule := range Builtin() {
		var matches, misses int
		for _, tt := range cases[rule.ID] {
			if tt.want == "" {
				misses++
				continue
			}
			matches++
		}
		if matches == 0 || misses == 0 {
			t.Errorf("rule %s has %d matching and %d near-miss cases, want at least one of each", rule.ID, matches, misses)
		}
	}
	for id := range cases {
		if !hasRule(Builtin(), id) {
			t.Errorf("cases for unknown rule %s", id)
		}
	}
}

func hasRule(rules []detect.Rule, id string) bool {
	for _, r := range rules {
		if r.ID == id {
			return true
		}
	}

	return false
}

func TestSecretOverlaps(t *testing.T) {
	t.Parallel()

	tests := []ruleCase{
		{"vendor key in .env: one token, the longer secret-assignment label", "STRIPE_SECRET=sk_live_" + fake(24), "STRIPE_SECRET=<SECRET_1>"},
		{"APP_KEY wins over secret-assignment on the same span", "APP_KEY=base64:" + fake(43) + "=", "APP_KEY=<APP_KEY_1>"},
		{"JWT in an authorization header", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0." + fake(43), "Authorization: Bearer <AUTH_1>"},
		{"same value, same token across rules", "DB_PASSWORD=hunter2\nmysql://app:hunter2@db/shop", "DB_PASSWORD=<SECRET_1>\nmysql://app:<SECRET_1>@db/shop"},
		{"cleaning twice changes nothing", "DB_PASSWORD=<SECRET_1>\nAuthorization: Bearer <AUTH_1>", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			want := tt.want
			if want == "" {
				want = tt.in
			}
			if got := clean(t, tt.in); got != want {
				t.Errorf("clean(%q)\n got %q\nwant %q", tt.in, got, want)
			}
		})
	}
}

func TestNotReference(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]bool{
		"hunter2": true, "": false, "  ": false, "NULL": false, "false": false,
		"${DB_PASSWORD}": false, "$DB_PASSWORD": false, "{{ secret }}": false, "%APPDATA%": false,
		"<SECRET_1>": false, "<secret>": true, "$5ecret": true,
	} {
		if got := notReference(value); got != want {
			t.Errorf("notReference(%q) = %v, want %v", value, got, want)
		}
	}
}
