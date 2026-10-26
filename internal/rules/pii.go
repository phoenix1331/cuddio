package rules

import (
	"net/netip"
	"regexp"
	"strings"
	"unicode"

	"github.com/phoenix1331/cuddio/internal/detect"
)

// ipPattern finds IPv4 and IPv6 candidates, which netip then validates. The
// character before an address may not be a word character, "." or a single
// "/", so versions in user agents ("Chrome/130.0.0.0") are skipped while
// URLs ("http://203.0.113.9/") are not. IPv6 candidates must contain a
// digit, so PHP calls such as "Feed::add" are not addresses.
//
// Nothing after the address is consumed (IPv4 ends at \b; netip rejects a
// cut-off IPv6 address), so the separator after one address can still be the
// leading context of the next: "203.0.113.9 203.0.113.10" finds both.
const ipPattern = `(?:^|[^\w./:]|//)(?P<value>(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b|(?i:[0-9a-f]{0,4}:){2,7}(?i:[0-9a-f]{0,4}))`

func pii() []detect.Rule {
	return inCategory(detect.PII, []detect.Rule{
		{
			// An address may not follow ":" (except in mailto:), so the
			// password and host in user:password@host are not an email.
			// "%40" is "@" URL-encoded, as in reset links (?email=a%40b.com)
			ID:       "email",
			Label:    "EMAIL",
			Pattern:  regexp.MustCompile(`(?:^|[^\w.%+:-]|(?i:mailto:))(?P<value>[A-Za-z0-9._%+-]+(?:@|%40)(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63})\b`),
			Keywords: []string{"@", "%40"},
			Validate: emailAddress,
		},
		{
			// 0 or +44 (optionally "(0)"), then 9 or 10 digits with single
			// spaces, hyphens or brackets between them; never after a word
			// character or "+", so "+0123456789" is not a UK number
			ID:       "uk-phone",
			Label:    "PHONE",
			Pattern:  regexp.MustCompile(`(?:^|[^\w+(])(?P<value>(?:\+44[ -]?(?:\(0\)[ -]?)?|0|\(0)[1-9](?:[ ()-]{0,2}[0-9]){8,9})\b`),
			Validate: ukPhone,
		},
		{
			ID:       "e164-phone",
			Label:    "PHONE",
			Pattern:  regexp.MustCompile(`(?:^|[^\w+])(?P<value>\+[1-9](?:[ -]?[0-9]){6,14})\b`),
			Keywords: []string{"+"},
			Validate: digitCount(7, 15),
		},
		{
			ID:       "ip-address",
			Label:    "IP",
			Pattern:  regexp.MustCompile(ipPattern),
			Validate: publicIP,
		},
		{
			ID:       "private-ip-address",
			Label:    "IP",
			Pattern:  regexp.MustCompile(ipPattern),
			Validate: privateIP,
			Optional: true,
		},
		{
			ID:       "payment-card",
			Label:    "CARD",
			Pattern:  regexp.MustCompile(`\b[0-9](?:[ -]?[0-9]){12,18}\b`),
			Validate: paymentCard,
		},
		{
			// Prefix letters exclude D, F, I, Q, U and V (and O second); the
			// suffix is A to D
			ID:       "uk-ni-number",
			Label:    "NI_NUMBER",
			Pattern:  regexp.MustCompile(`\b[A-CEGHJ-PR-TW-Z][A-CEGHJ-NPR-TW-Z] ?[0-9]{2} ?[0-9]{2} ?[0-9]{2} ?[A-D]\b`),
			Validate: niPrefix,
		},
		{
			// Off by default: many short codes in logs look like postcodes
			ID:       "uk-postcode",
			Label:    "POSTCODE",
			Pattern:  regexp.MustCompile(`\b(?:GIR ?0AA|(?:[A-PR-UWYZ][0-9]{1,2}|[A-PR-UWYZ][A-HK-Y][0-9]{1,2}|[A-PR-UWYZ][0-9][A-HJKPSTUW]|[A-PR-UWYZ][A-HK-Y][0-9][ABEHMNPRV-Y]) ?[0-9][ABD-HJLNP-UW-Z]{2})\b`),
			Optional: true,
		},
		{
			// Only the username is hidden, so paths stay readable. Windows
			// usernames may contain spaces; JSON escapes backslashes as "\\"
			ID:       "home-directory",
			Label:    "USER",
			Pattern:  regexp.MustCompile(`(?:/(?:home|Users)/(?P<value>[^/\s"':]+)|(?i:\b[A-Z]:(?:\\|/)+Users(?:\\|/)+)(?P<value>[^\\/"':*?<>|\r\n]*[^\\/"':*?<>|\s]))`),
			Keywords: []string{"/home/", "users"},
			Validate: realUser,
		},
	})
}

// fileExtensions look like top-level domains in names such as logo@2x.png.
var fileExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true, "webp": true, "avif": true, "ico": true,
	"css": true, "js": true, "ts": true, "php": true, "html": true, "json": true, "map": true,
}

// emailAddress rejects file names such as logo@2x.png and SSH remotes such
// as git@github.com, which use the git user rather than a person's address.
func emailAddress(email string) bool {
	tld := strings.ToLower(email[strings.LastIndexByte(email, '.')+1:])
	return !fileExtensions[tld] && !strings.HasPrefix(email, "git@") && !strings.HasPrefix(email, "git%40")
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func digitCount(minimum, maximum int) func(string) bool {
	return func(s string) bool {
		n := len(digits(s))
		return n >= minimum && n <= maximum
	}
}

// ukPhone checks the number of digits: 10 or 11 after a leading 0, or 9 or
// 10 after +44, ignoring a "(0)".
func ukPhone(s string) bool {
	d := digits(s)
	if national, ok := strings.CutPrefix(d, "44"); ok && strings.HasPrefix(s, "+") {
		if strings.Contains(s, "(0)") {
			national = strings.TrimPrefix(national, "0")
		}
		return len(national) == 9 || len(national) == 10
	}

	return len(d) == 10 || len(d) == 11
}

// cgnat is the carrier-grade NAT range, shared address space that is not
// public.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func parseIP(s string) (netip.Addr, bool) {
	if !strings.ContainsAny(s, "0123456789") {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}

	return addr.Unmap(), true
}

func isPrivate(a netip.Addr) bool {
	return a.IsPrivate() || a.IsLinkLocalUnicast() || cgnat.Contains(a)
}

// publicIP accepts globally routable unicast addresses. Loopback, multicast,
// unspecified and private addresses are left to private-ip-address or kept.
func publicIP(s string) bool {
	a, ok := parseIP(s)
	return ok && a.IsGlobalUnicast() && !isPrivate(a)
}

// privateIP accepts RFC 1918, unique local, link-local and CGNAT addresses.
func privateIP(s string) bool {
	a, ok := parseIP(s)
	return ok && isPrivate(a)
}

// paymentCard accepts 13 to 19 digits with a known card prefix (Visa,
// Mastercard, Amex, Discover) that pass the Luhn check. The prefix check
// keeps millisecond timestamps and order numbers out.
func paymentCard(s string) bool {
	d := digits(s)
	return len(d) >= 13 && len(d) <= 19 && cardPrefix.MatchString(d) && detect.Luhn(d)
}

var cardPrefix = regexp.MustCompile(`^(?:4|5[1-5]|2[2-7]|3[47]|6(?:011|5|4[4-9]))`)

// niPrefix rejects prefixes HMRC never issues.
func niPrefix(s string) bool {
	switch s[:2] {
	case "BG", "GB", "NK", "KN", "TN", "NT", "ZZ":
		return false
	}

	return true
}

// sharedProfiles are home directories that do not name a person.
var sharedProfiles = map[string]bool{
	"shared": true, "public": true, "default": true, "default user": true, "all users": true,
}

func realUser(name string) bool {
	return notReference(name) && !sharedProfiles[strings.ToLower(name)]
}
