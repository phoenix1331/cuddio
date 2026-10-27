package replace

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Fakes hands out realistic but obviously fake values, so a cleaned dump
// still imports and an app still runs against it: the same value always
// gets the same fake, so joins on an email still line up. Each fake comes
// from its label and a per-label counter, never from the value it replaces.
// Use one per run; it is not safe for concurrent use.
type Fakes struct {
	byValue map[string]string
	counts  map[string]int
}

// NewFakes returns a fake value generator for one run.
func NewFakes() *Fakes {
	return &Fakes{byValue: map[string]string{}, counts: map[string]int{}}
}

// Token returns the fake for value. A value seen before keeps its first
// fake, even if a different rule (and label) matches it later. A fake that
// equals value (a published test card fed back in) is skipped for the next
// one; only fixed fakes such as REDACTED can come out unchanged.
func (f *Fakes) Token(label, value string) string {
	if fake, ok := f.byValue[value]; ok {
		return fake
	}

	var fake string
	for range maxSkips {
		f.counts[label]++
		if fake = fakeFor(label, value, f.counts[label]); fake != value {
			break
		}
	}
	f.byValue[value] = fake

	return fake
}

// maxSkips bounds how many fakes Token tries for one value: enough to get
// past a match in any generator that varies with its counter.
const maxSkips = 3

// IsToken reports false: a fake is meant to pass for a real value, so it
// cannot be told apart from one.
func (f *Fakes) IsToken(string) bool {
	return false
}

// Fit shortens fake to at most maxLen characters, the length of the column
// it goes in. A maxLen of 0 means no limit.
func (f *Fakes) Fit(fake string, maxLen int) string {
	if maxLen <= 0 {
		return fake
	}
	runes := []rune(fake)
	if len(runes) <= maxLen {
		return fake
	}

	return string(runes[:maxLen])
}

// labelIs reports whether label is one of kinds or ends in "_" plus one,
// so BILLING_NAME is a name and SMTP_PASSWORD a password.
func labelIs(label string, kinds ...string) bool {
	for _, kind := range kinds {
		if label == kind || strings.HasSuffix(label, "_"+kind) {
			return true
		}
	}

	return false
}

// fakeFor returns the nth fake for label, shaped like value where the kind
// of value has a recognisable format.
func fakeFor(label, value string, n int) string {
	switch {
	case labelIs(label, "EMAIL"):
		return "user" + strconv.Itoa(n) + "@example.invalid"
	case labelIs(label, "PHONE", "MOBILE"):
		return fakePhone(value, n)
	case labelIs(label, "IP", "IP_ADDRESS"):
		return fakeIP(value, n)
	case labelIs(label, "CARD"):
		return fakeCard(value, n)
	case labelIs(label, "NI_NUMBER"):
		return fakeNI(value, n)
	case labelIs(label, "POSTCODE"):
		return fakePostcode(value, n)
	case labelIs(label, "DOB", "DATE_OF_BIRTH"):
		return fakeDate(value, n)
	case labelIs(label, "ADDRESS"):
		return strconv.Itoa(n) + " Example Street, Testville"
	case labelIs(label, "NAME"):
		return fakeName(value, n)
	case labelIs(label, "USER"):
		return "user" + strconv.Itoa(n)
	case labelIs(label, "PASSWORD", "PRIVATE_KEY"):
		return "REDACTED"
	}

	return scramble(label, value, n)
}

// dramaPrefixes are the first eight digits of Ofcom's ranges reserved for
// drama, each followed by three free digits: 07700 900xxx, 01632 960xxx,
// 020 7946 0xxx and the 0xxx 496 0xxx geographic ranges.
var dramaPrefixes = []string{
	"07700900", "01632960", "02079460", "01134960", "01144960", "01154960", "01164960",
	"01174960", "01184960", "01214960", "01314960", "01414960", "01514960", "01614960", "02920180",
}

// fakePhone returns a number from Ofcom's drama ranges, in international
// form when value is, and laid out like value when the digit counts match.
func fakePhone(value string, n int) string {
	national := dramaPrefixes[(n/1000)%len(dramaPrefixes)] + pad(n%1000, 3)
	trimmed := strings.TrimSpace(value)
	digits := national
	switch {
	case strings.HasPrefix(trimmed, "+44"):
		digits = "44" + national[1:]
	case strings.HasPrefix(trimmed, "0044"):
		digits = "0044" + national[1:]
	}
	if shaped, ok := fill(value, digits, isASCIIDigit); ok {
		return shaped
	}
	if strings.HasPrefix(digits, "44") {
		return "+" + digits
	}

	return digits
}

// docRanges are the IPv4 documentation ranges (RFC 5737).
var docRanges = []string{"192.0.2.", "198.51.100.", "203.0.113."}

// fakeIP returns an address from the documentation ranges: IPv4 for IPv4,
// 2001:db8::/32 (RFC 3849) for IPv6.
func fakeIP(value string, n int) string {
	if strings.Contains(value, ":") {
		return "2001:db8::" + strconv.FormatInt(int64(n), 16)
	}
	i := n - 1

	return docRanges[(i/254)%len(docRanges)] + strconv.Itoa(i%254+1)
}

// testCards are card numbers payment providers publish for testing, by
// length. None can be charged.
var testCards = map[int][]string{
	14: {"36227206271667"},
	15: {"378282246310005", "371449635398431"},
	16: {"4242424242424242", "4000056655665556", "5555555555554444", "2223003122003222", "5200828282828210", "5105105105105100", "6011111111111117", "6011000990139424"},
}

// fakeCard returns a published test card number with as many digits as
// value, laid out like value.
func fakeCard(value string, n int) string {
	cards, ok := testCards[countOf(value, isASCIIDigit)]
	if !ok {
		cards = testCards[16]
	}
	card := cards[(n-1)%len(cards)]
	if shaped, ok := fill(value, card, isASCIIDigit); ok {
		return shaped
	}

	return card
}

// fakeNI returns a National Insurance number with the QQ prefix, which
// HMRC never issues, laid out like value.
func fakeNI(value string, n int) string {
	ni := "QQ" + pad(n%1000000, 6) + "C"
	if shaped, ok := fill(value, ni, isASCIIAlnum); ok {
		return shaped
	}

	return ni
}

// fakePostcode returns a postcode in the ZZ99 area, which no address uses,
// laid out like value when it has the usual seven letters and digits.
func fakePostcode(value string, n int) string {
	letters := (n / 10) % (26 * 26)
	inward := strconv.Itoa(n%10) + string(rune('A'+letters/26)) + string(rune('A'+letters%26))
	if shaped, ok := fill(value, "ZZ99"+inward, isASCIIAlnum); ok {
		return shaped
	}

	return "ZZ99 " + inward
}

// fakeDate returns a date between 1950 and 2004 in value's layout:
// YYYY-MM-DD or DD/MM/YYYY, keeping anything after it, such as a time.
func fakeDate(value string, n int) string {
	d := time.Date(1950, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, (n*97)%20000)
	switch {
	case len(value) >= 10 && value[4] == '-' && value[7] == '-':
		return d.Format("2006-01-02") + value[10:]
	case len(value) >= 10 && value[2] == '/' && value[5] == '/':
		return d.Format("02/01/2006") + value[10:]
	}

	return d.Format("2006-01-02")
}

var (
	firstNames = []string{"Alex", "Sam", "Jordan", "Taylor", "Morgan", "Casey", "Riley", "Jamie", "Avery", "Quinn", "Rowan", "Elliot", "Harper", "Reese", "Dylan", "Ffion", "Rhys", "Megan", "Owen", "Cerys"}
	lastNames  = []string{"Smith", "Jones", "Williams", "Davies", "Evans", "Thomas", "Roberts", "Hughes", "Lewis", "Morgan", "Price", "Griffiths", "Owen", "Edwards", "Jenkins", "James", "Phillips", "Bennett", "Walker", "Wright"}
)

// fakeName returns a plausible name: a first name for a one-word value,
// otherwise a first name and surname. Both change from one fake to the
// next, and the first 400 full names are all different.
func fakeName(value string, n int) string {
	i := n - 1
	first := firstNames[i%len(firstNames)]
	if len(strings.Fields(value)) <= 1 {
		return first
	}

	return first + " " + lastNames[(3*i+i/len(firstNames))%len(lastNames)]
}

// scramble keeps value's shape (length, and upper case, lower case and
// digits where they were) with characters from a generator seeded by label
// and n. Other characters, such as "_" and "-", are kept.
func scramble(label, value string, n int) string {
	// FNV-1a over label and n seeds an xorshift generator
	state := uint64(14695981039346656037)
	for _, c := range []byte(label + ":" + strconv.Itoa(n)) {
		state = (state ^ uint64(c)) * 1099511628211
	}

	out := []rune(value)
	for i, r := range out {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		pick := rune(state % 26)
		switch {
		case unicode.IsUpper(r):
			out[i] = 'A' + pick
		case unicode.IsLetter(r):
			out[i] = 'a' + pick
		case unicode.IsDigit(r):
			out[i] = '0' + pick%10
		}
	}
	fake := string(out)
	if fake == value && fake != "" {
		// A short value can come out unchanged; change its first character
		return string(rotate(out[0])) + string(out[1:])
	}

	return fake
}

// rotate moves r on by one within its class, so it is never itself.
func rotate(r rune) rune {
	switch {
	case r == 'z', r == 'Z':
		return r - 25
	case r == '9':
		return '0'
	case unicode.IsLetter(r), unicode.IsDigit(r):
		return r + 1
	}

	return 'x'
}

// fill writes chars into template, replacing each character that ok
// accepts in order, so a fake keeps the original's spaces, dashes and
// brackets. It reports false when the counts differ.
func fill(template, chars string, ok func(rune) bool) (string, bool) {
	if countOf(template, ok) != len(chars) {
		return "", false
	}

	out := []rune(template)
	next := 0
	for i, r := range out {
		if ok(r) {
			out[i] = rune(chars[next])
			next++
		}
	}

	return string(out), true
}

func countOf(s string, ok func(rune) bool) int {
	n := 0
	for _, r := range s {
		if ok(r) {
			n++
		}
	}

	return n
}

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

func isASCIIAlnum(r rune) bool {
	return isASCIIDigit(r) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// pad writes n with leading zeros to width digits.
func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}

	return s
}
