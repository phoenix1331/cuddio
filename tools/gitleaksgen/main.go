// Command gitleaksgen turns gitleaks' default config into Go source for
// cuddio's vendored rules, so the binary needs no TOML parser and compiles
// no more than it uses. scripts/update-gitleaks-rules.sh runs it.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"regexp"
	"regexp/syntax"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "gitleaksgen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	flags := flag.NewFlagSet("gitleaksgen", flag.ContinueOnError)
	in := flags.String("in", "", "path to gitleaks.toml")
	version := flags.String("version", "", "gitleaks release tag the file came from")
	sum := flags.String("sha256", "", "SHA-256 of the file, recorded in the output")
	exclude := flags.String("exclude", "", "comma-separated rule IDs to leave out")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *in == "" || *version == "" || *sum == "" {
		return fmt.Errorf("-in, -version and -sha256 are required")
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	cfg, err := decode(data)
	if err != nil {
		return err
	}

	var excluded []string
	if *exclude != "" {
		excluded = strings.Split(*exclude, ",")
	}
	out, err := generate(cfg, header{version: *version, sum: *sum, excluded: excluded})
	if err != nil {
		return err
	}
	_, err = w.Write(out)

	return err
}

type allowlist struct {
	Condition   string   `toml:"condition"`
	RegexTarget string   `toml:"regexTarget"`
	Regexes     []string `toml:"regexes"`
	Stopwords   []string `toml:"stopwords"`
	Paths       []string `toml:"paths"`
	Commits     []string `toml:"commits"`
	Description string   `toml:"description"`
}

type rule struct {
	ID          string      `toml:"id"`
	Description string      `toml:"description"`
	Regex       string      `toml:"regex"`
	SecretGroup int         `toml:"secretGroup"`
	Entropy     float64     `toml:"entropy"`
	Keywords    []string    `toml:"keywords"`
	Path        string      `toml:"path"`
	Tags        []string    `toml:"tags"`
	Allowlists  []allowlist `toml:"allowlists"`
}

type config struct {
	Title      string    `toml:"title"`
	MinVersion string    `toml:"minVersion"`
	Allowlist  allowlist `toml:"allowlist"`
	Rules      []rule    `toml:"rules"`
}

// decode parses the config and fails on any key this generator does not
// know, so an upstream feature is never silently dropped.
func decode(data []byte) (config, error) {
	var cfg config
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return config{}, fmt.Errorf("unsupported keys: %v", undecoded)
	}

	return cfg, nil
}

type header struct {
	version  string
	sum      string
	excluded []string
}

// vendored is one rule as cuddio needs it.
type vendored struct {
	id        string
	pattern   string
	keywords  []string
	entropy   float64
	regexes   []string
	stopwords []string
}

func generate(cfg config, h header) ([]byte, error) {
	global, err := reduce("global allowlist", []allowlist{cfg.Allowlist})
	if err != nil {
		return nil, err
	}

	var rules []vendored
	var pathScoped []string
	for _, r := range cfg.Rules {
		if slices.Contains(h.excluded, r.ID) {
			continue
		}
		// Path-scoped rules need a file name; stdin and the clipboard have none
		if r.Path != "" {
			pathScoped = append(pathScoped, r.ID)
			continue
		}
		v, err := convert(r)
		if err != nil {
			return nil, err
		}
		rules = append(rules, v)
	}
	slices.SortFunc(rules, func(a, b vendored) int { return strings.Compare(a.id, b.id) })

	var b bytes.Buffer
	writeHeader(&b, h, pathScoped)
	fmt.Fprintf(&b, "const gitleaksVersion = %q\n\n", h.version)
	b.WriteString("func gitleaksGlobalAllowlist() vendoredAllowlist {\n\treturn ")
	writeAllowlist(&b, global)
	b.WriteString("\n}\n\nfunc gitleaksRules() []vendoredRule {\n\treturn []vendoredRule{\n")
	for _, v := range rules {
		fmt.Fprintf(&b, "{\nID: %q,\nPattern: %s,\nKeywords: %s,\n", v.id, quote(v.pattern), stringSlice(v.keywords))
		if v.entropy > 0 {
			fmt.Fprintf(&b, "Entropy: %s,\n", strconv.FormatFloat(v.entropy, 'g', -1, 64))
		}
		if len(v.regexes) > 0 || len(v.stopwords) > 0 {
			b.WriteString("Allow: ")
			writeAllowlist(&b, v)
			b.WriteString(",\n")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n}\n")

	return format.Source(b.Bytes())
}

// convert checks one rule and renames its secret capture groups to "value".
func convert(r rule) (vendored, error) {
	if r.Regex == "" {
		return vendored{}, fmt.Errorf("rule %s: no regex", r.ID)
	}
	pattern, err := nameSecretGroups(r.Regex, r.SecretGroup)
	if err != nil {
		return vendored{}, fmt.Errorf("rule %s: %w", r.ID, err)
	}
	if len(r.Keywords) == 0 {
		return vendored{}, fmt.Errorf("rule %s: no keywords", r.ID)
	}
	for _, k := range r.Keywords {
		if k != strings.ToLower(k) {
			return vendored{}, fmt.Errorf("rule %s: keyword %q is not lowercase", r.ID, k)
		}
	}
	allow, err := reduce("rule "+r.ID, r.Allowlists)
	if err != nil {
		return vendored{}, err
	}

	return vendored{
		id:        r.ID,
		pattern:   pattern,
		keywords:  r.Keywords,
		entropy:   r.Entropy,
		regexes:   allow.regexes,
		stopwords: allow.stopwords,
	}, nil
}

// nameSecretGroups names the capture groups gitleaks takes the secret from
// "value": group n when secretGroup is set, otherwise every group, since
// gitleaks uses the first non-empty one and cuddio the first that took part.
// The pattern text is edited in place so it stays as readable as upstream,
// then both versions are parsed to prove only group names changed.
func nameSecretGroups(pattern string, secretGroup int) (string, error) {
	var b strings.Builder
	group, found := 0, false
	inClass := false

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		b.WriteByte(c)
		switch {
		case c == '\\' && i+1 < len(pattern):
			i++
			b.WriteByte(pattern[i])
		case inClass:
			if c == '[' && strings.HasPrefix(pattern[i:], "[:") {
				end := strings.Index(pattern[i:], ":]")
				if end > 0 {
					b.WriteString(pattern[i+1 : i+end+2])
					i += end + 1
				}
				continue
			}
			inClass = c != ']'
		case c == '[':
			inClass = true
			// A ] straight after [ or [^ is a literal
			if strings.HasPrefix(pattern[i+1:], "^") {
				i++
				b.WriteByte('^')
			}
			if strings.HasPrefix(pattern[i+1:], "]") {
				i++
				b.WriteByte(']')
			}
		case c == '(':
			name, skip, capture := groupName(pattern[i+1:])
			if !capture {
				continue
			}
			group++
			if secretGroup == 0 || group == secretGroup {
				b.WriteString("?P<value>")
				found = true
			} else if name {
				b.WriteString(pattern[i+1 : i+1+skip])
			}
			i += skip
		}
	}
	if secretGroup > 0 && !found {
		return "", fmt.Errorf("secret group %d does not exist", secretGroup)
	}

	out := b.String()
	if err := sameExceptNames(pattern, out); err != nil {
		return "", err
	}

	return out, nil
}

// groupName looks at the text after "(" and reports whether it starts a
// capture group, whether that group is already named, and how many bytes
// the name syntax ("?P<x>" or "?<x>") takes.
func groupName(rest string) (named bool, skip int, capture bool) {
	for _, prefix := range []string{"?P<", "?<"} {
		if strings.HasPrefix(rest, prefix) {
			end := strings.IndexByte(rest, '>')
			return true, end + 1, true
		}
	}

	return false, 0, !strings.HasPrefix(rest, "?")
}

// sameExceptNames fails unless renamed parses to the same tree as original
// once group names are cleared.
func sameExceptNames(original, renamed string) error {
	a, err := syntax.Parse(original, syntax.Perl)
	if err != nil {
		return err
	}
	b, err := syntax.Parse(renamed, syntax.Perl)
	if err != nil {
		return fmt.Errorf("renaming groups broke the pattern: %w", err)
	}
	clearNames(a)
	clearNames(b)
	if !a.Equal(b) {
		return fmt.Errorf("renaming groups changed the pattern")
	}

	return nil
}

func clearNames(n *syntax.Regexp) {
	n.Name = ""
	for _, sub := range n.Sub {
		clearNames(sub)
	}
}

// reduce keeps the parts of allowlists that apply to text with no file path
// or commit: regexes against the secret and stopwords. An allowlist whose
// AND condition needs a path or commit can never apply, so it is dropped.
func reduce(name string, lists []allowlist) (vendored, error) {
	var v vendored
	for _, a := range lists {
		if a.Condition != "" && a.Condition != "OR" && a.Condition != "AND" {
			return vendored{}, fmt.Errorf("%s: unknown condition %q", name, a.Condition)
		}
		if a.Condition == "AND" && (len(a.Paths) > 0 || len(a.Commits) > 0) {
			continue
		}
		if len(a.Regexes) > 0 && a.RegexTarget != "" && a.RegexTarget != "secret" {
			return vendored{}, fmt.Errorf("%s: regexTarget %q is not supported; exclude the rule or extend cuddio", name, a.RegexTarget)
		}
		for _, re := range a.Regexes {
			if _, err := regexp.Compile(re); err != nil {
				return vendored{}, fmt.Errorf("%s: %w", name, err)
			}
		}
		v.regexes = append(v.regexes, a.Regexes...)
		for _, s := range a.Stopwords {
			v.stopwords = append(v.stopwords, strings.ToLower(s))
		}
	}

	return v, nil
}

func writeHeader(b *bytes.Buffer, h header, pathScoped []string) {
	fmt.Fprintf(b, `// Code generated by tools/gitleaksgen; DO NOT EDIT.

// Source: https://github.com/gitleaks/gitleaks/blob/%[1]s/config/gitleaks.toml
// gitleaks %[1]s, SHA-256 %[2]s
// Regenerate with scripts/update-gitleaks-rules.sh.
//
// Left out: %[3]s (by choice); %[4]s (path-scoped).
//
// The rules below are derived from gitleaks, used under the MIT licence:
//
// Copyright (c) 2019 Zachary Rice
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package rules

`, h.version, h.sum, orNone(h.excluded), orNone(pathScoped))
}

func writeAllowlist(b *bytes.Buffer, v vendored) {
	b.WriteString("vendoredAllowlist{")
	if len(v.regexes) > 0 {
		b.WriteString("\nRegexes: []string{\n")
		for _, re := range v.regexes {
			fmt.Fprintf(b, "%s,\n", quote(re))
		}
		b.WriteString("},")
	}
	if len(v.stopwords) > 0 {
		fmt.Fprintf(b, "\nStopwords: %s,\n", stringSlice(v.stopwords))
	}
	b.WriteString("}")
}

// quote writes s as a raw string literal where it can, so patterns stay
// readable.
func quote(s string) string {
	if strings.ContainsAny(s, "`\r") {
		return strconv.Quote(s)
	}

	return "`" + s + "`"
}

func stringSlice(s []string) string {
	quoted := make([]string, len(s))
	for i, v := range s {
		quoted[i] = strconv.Quote(v)
	}

	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}

	return strings.Join(s, ", ")
}
