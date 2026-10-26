package rules

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/phoenix1331/cuddio/internal/detect"
)

// Shared pattern pieces. A value group stops at a quote or line end, so the
// quotes around a value stay in place.
const (
	// quotedValue matches a double- or single-quoted value, or a bare one up
	// to whitespace or a comment.
	quotedValue = `(?:"(?P<value>(?:[^"\\\r\n]|\\.)*)"|'(?P<value>[^'\r\n]*)'|(?P<value>[^\s#"']\S*))`
	// restOfValue matches to the end of the line or the closing quote,
	// without trailing whitespace.
	restOfValue = `(?P<value>[^"'\r\n]*[^"'\s])`
	// secretKeyName matches key names that suggest a secret.
	secretKeyName = `(?:KEY|SECRET|TOKEN|PASSWORD|PASS|PRIVATE)`
)

func secrets() []detect.Rule {
	return inCategory(detect.Secret, []detect.Rule{
		{
			ID:      "aws-access-key",
			Label:   "AWS_KEY",
			Pattern: regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}\b`),
		},
		{
			ID:       "aws-secret-key",
			Label:    "AWS_SECRET",
			Pattern:  regexp.MustCompile(`(?i)aws_?secret_?(?:access_?)?key["']?\s*(?:[:=]|=>)\s*["']?(?P<value>[A-Za-z0-9/+]{40})(?:[^A-Za-z0-9/+=]|$)`),
			Validate: notReference,
		},
		{
			ID:      "github-token",
			Label:   "GITHUB_TOKEN",
			Pattern: regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59})\b`),
		},
		{
			ID:      "stripe-key",
			Label:   "STRIPE_KEY",
			Pattern: regexp.MustCompile(`\b(?:(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{10,247}|whsec_[A-Za-z0-9]{24,})\b`),
		},
		{
			ID:      "slack-token",
			Label:   "SLACK_TOKEN",
			Pattern: regexp.MustCompile(`\b(?:xox[abposr]-[A-Za-z0-9-]{10,250}|xapp-[0-9]-[A-Za-z0-9-]{10,250})`),
		},
		{
			ID:      "slack-webhook",
			Label:   "SLACK_WEBHOOK",
			Pattern: regexp.MustCompile(`https://hooks\.slack\.com/(?:services|workflows|triggers)/[A-Za-z0-9_/+-]{20,}`),
		},
		{
			ID:      "openai-key",
			Label:   "OPENAI_KEY",
			Pattern: regexp.MustCompile(`\bsk-(?:(?:proj|svcacct|admin)-[A-Za-z0-9_-]{20,}|[A-Za-z0-9]{20}T3BlbkFJ[A-Za-z0-9]{20})`),
		},
		{
			ID:      "anthropic-key",
			Label:   "ANTHROPIC_KEY",
			Pattern: regexp.MustCompile(`\bsk-ant-(?:api|admin|sid)[0-9]{2}-[A-Za-z0-9_-]{20,}`),
		},
		{
			ID:      "google-api-key",
			Label:   "GOOGLE_KEY",
			Pattern: regexp.MustCompile(`\b(?:AIza[0-9A-Za-z_-]{35}|GOCSPX-[0-9A-Za-z_-]{28})`),
		},
		{
			ID:      "jwt",
			Label:   "JWT",
			Pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*`),
		},
		{
			// The scheme (Bearer, Basic, ...) stays visible; everything after it is hidden
			ID:       "authorization-header",
			Label:    "AUTH",
			Pattern:  regexp.MustCompile(`(?i)\b(?:proxy-)?authorization["']?\s*[:=]\s*["']?(?:(?:bearer|basic|token|digest|negotiate|ntlm)\s+)?` + restOfValue),
			Validate: notReference,
		},
		{
			ID:      "bearer-token",
			Label:   "TOKEN",
			Pattern: regexp.MustCompile(`(?i)\bbearer\s+(?P<value>[A-Za-z0-9._~+/-]{16,}=*)`),
		},
		{
			ID:       "cookie",
			Label:    "COOKIE",
			Pattern:  regexp.MustCompile(`(?i)\b(?:set-)?cookie["']?\s*[:=]\s*["']?` + restOfValue),
			Validate: notReference,
		},
		{
			ID:       "url-password",
			Label:    "PASSWORD",
			Pattern:  regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9+.-]*://[^\s:@/?#]+:(?P<value>[^\s@/?#]+)@`),
			Validate: notReference,
		},
		{
			ID:       "token-query-param",
			Label:    "TOKEN",
			Pattern:  regexp.MustCompile(`(?i)[?&](?:access_token|refresh_token|id_token|token|api_key|apikey|api-key|key|secret|client_secret|signature|sig|password|passwd|pwd|auth|code|session|sessionid|x-amz-signature|x-amz-credential|x-amz-security-token|x-goog-signature|x-goog-credential)=(?P<value>[^&\s#"'<>]+)`),
			Validate: notReference,
		},
		{
			// .env style, commented-out lines included: an old key is still a key
			ID:       "secret-assignment",
			Label:    "SECRET",
			Pattern:  regexp.MustCompile(`(?im)^[ \t]*(?:#[ \t]*)?(?:export[ \t]+)?[A-Z0-9_.]*` + secretKeyName + `[A-Z0-9_.]*[ \t]*=[ \t]*` + quotedValue),
			Validate: notReference,
		},
		{
			// JSON and PHP array context, as in Laravel log payloads
			ID:       "secret-field",
			Label:    "SECRET",
			Pattern:  regexp.MustCompile(`(?i)["'][A-Za-z0-9_.-]*(?:password|passwd|secret|token|api_?key|private_?key|credential)[A-Za-z0-9_.-]*["']\s*(?::|=>)\s*(?:"(?P<value>(?:[^"\\\r\n]|\\.)+)"|'(?P<value>[^'\r\n]+)')`),
			Validate: notReference,
		},
		{
			ID:      "laravel-app-key",
			Label:   "APP_KEY",
			Pattern: regexp.MustCompile(`\bbase64:[A-Za-z0-9+/]{43}=`),
		},
		{
			// An unterminated block (cut off, or truncated in a log) is hidden to its end
			ID:      "private-key",
			Label:   "PRIVATE_KEY",
			Pattern: regexp.MustCompile(`-----BEGIN[A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----(?s:.*?)(?:-----END[A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----|\z)`),
		},
		{
			// Precision over recall: long, mixed-case, with digits, and random-looking.
			// The value cannot start with "/", so "https://..." is not a key and value
			ID:       "high-entropy-assignment",
			Label:    "SECRET",
			Pattern:  regexp.MustCompile(`(?i)\b[a-z_][a-z0-9_.-]*["']?[ \t]*(?:[:=]|=>)[ \t]*["']?(?P<value>[A-Za-z0-9+][A-Za-z0-9+/_~.-]{31,}={0,2})`),
			Validate: highEntropy,
		},
	})
}

// references are values that name a secret rather than hold one.
var references = regexp.MustCompile(`(?i)^(?:null|nil|none|true|false|\$\{[^}]*\}|\$[A-Z_][A-Z0-9_]*|\{\{.*\}\}|%[A-Z_]+%|<[A-Z][A-Z0-9_]*_[0-9]+>)$`)

// notReference rejects empty values, booleans, nulls, environment
// references such as ${DB_PASSWORD}, and placeholders cuddio wrote itself,
// so cleaning output twice leaves it unchanged.
func notReference(value string) bool {
	return strings.TrimSpace(value) != "" && !references.MatchString(value)
}

// highEntropyBits is the minimum entropy for high-entropy-assignment. Hex
// (git hashes, UUIDs) tops out at 4 bits per byte, so it never qualifies.
const highEntropyBits = 4.3

func highEntropy(value string) bool {
	var upper, lower, digit bool
	for _, r := range value {
		upper = upper || unicode.IsUpper(r)
		lower = lower || unicode.IsLower(r)
		digit = digit || unicode.IsDigit(r)
	}

	return upper && lower && digit && detect.Entropy(value) >= highEntropyBits
}
