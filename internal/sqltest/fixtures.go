package sqltest

import (
	"embed"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

//go:embed testdata/sql
var fixtureFS embed.FS

// Fixtures returns the fixture names in testdata/sql, sorted.
func Fixtures() []string {
	entries, _ := fs.ReadDir(fixtureFS, "testdata/sql")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	return names
}

// DialectOf returns the dialect of a fixture, from its name's prefix.
func DialectOf(name string) (Dialect, bool) {
	for _, d := range []Dialect{MySQL, Postgres, SQLite} {
		if strings.HasPrefix(name, string(d)) {
			return d, true
		}
	}

	return "", false
}

// Fixture returns a fixture dump with its template tags expanded.
func Fixture(tb testing.TB, name string) string {
	tb.Helper()

	data, err := fixtureFS.ReadFile(path.Join("testdata/sql", name))
	if err != nil {
		tb.Fatal(err)
	}

	return Expand(string(data))
}

const (
	fakeChars = "Ab3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1Wx"
	hexChars  = "0123456789abcdef"
)

// Fake returns n deterministic characters that look random: mixed case,
// digits and 5 bits per byte of entropy. Different seeds give different
// values, so fixtures can hold many distinct tokens.
func Fake(n, seed int) string {
	return cycle(fakeChars, n, seed*7)
}

// Hex returns n deterministic lowercase hex characters, like a SHA-256
// digest. Different seeds give different values.
func Hex(n, seed int) string {
	return cycle(hexChars, n, seed*5)
}

func cycle(chars string, n, offset int) string {
	var b strings.Builder
	b.Grow(n)
	for i := range n {
		b.WriteByte(chars[(i+offset)%len(chars)])
	}

	return b.String()
}

var templateTag = regexp.MustCompile(`\{\{(fake|hex) ([0-9]+)(?: ([0-9]+))?\}\}`)

// Expand fills in a fixture template. Tokens in vendor formats are written
// as {{fake N}} or {{hex N}}, with an optional seed ({{fake N S}}), so
// nothing committed looks like a live secret to GitHub push protection or a
// user's secret scanner.
func Expand(template string) string {
	return templateTag.ReplaceAllStringFunc(template, func(tag string) string {
		m := templateTag.FindStringSubmatch(tag)
		n, _ := strconv.Atoi(m[2])
		seed, _ := strconv.Atoi(m[3])
		if m[1] == "hex" {
			return Hex(n, seed)
		}
		return Fake(n, seed)
	})
}
