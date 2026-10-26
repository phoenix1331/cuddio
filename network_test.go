package main

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cuddio must never be able to send data anywhere. These tests check that at
// two levels: which packages the build imports, and which code the linker
// actually keeps in the binary.

// pureNetPackages parse or format network values without opening connections.
var pureNetPackages = map[string]bool{
	"net/netip": true,
	"net/url":   true,
}

// allowedNetImporters may import package net. spf13/pflag (used by cobra)
// imports it only to parse IP address flag values; the binary test below
// proves the linker drops every dial, listen and lookup function.
var allowedNetImporters = map[string]bool{
	"github.com/spf13/pflag": true,
}

// networkSymbol matches functions that open sockets, resolve names, or speak
// HTTP or TLS. None of them may survive into the binary.
var networkSymbol = regexp.MustCompile(`^(net\.(Dial|Listen|socket|internetSocket|sysSocket|\(\*Dialer\)|\(\*ListenConfig\)|\(\*Resolver\)|\(\*sysDialer\)|\(\*sysListener\))|net/http\.|crypto/tls\.|net/smtp\.|net/rpc\.)`)

func goTool(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is needed to inspect the build: %v", err)
	}

	return path
}

func TestBuildImportsNoNetworkPackages(t *testing.T) {
	t.Parallel()

	out, err := exec.Command(goTool(t), "list", "-deps", "-f", "{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	for _, problem := range networkImportProblems(string(out)) {
		t.Error(problem)
	}
}

// networkImportProblems reads go list lines of "package import import..." and
// reports every network package in the build that is not explicitly allowed.
func networkImportProblems(list string) []string {
	var problems []string

	scanner := bufio.NewScanner(strings.NewReader(list))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}

		importer, imports := fields[0], fields[1:]
		if isNetworkPackage(importer) {
			problems = append(problems, "network package in the build: "+importer)
		}
		for _, imported := range imports {
			if imported == "net" && !allowedNetImporters[importer] {
				problems = append(problems, importer+" imports net and is not in allowedNetImporters")
			}
		}
	}

	return problems
}

func isNetworkPackage(path string) bool {
	if path == "crypto/tls" {
		return true
	}
	if !strings.HasPrefix(path, "net/") {
		return false
	}

	return !pureNetPackages[path]
}

// testOnly lists packages and modules that only tests may use: SQL parsers
// and an SQLite engine for validity checks. They must never reach the binary.
var testOnly = []string{
	"github.com/phoenix1331/cuddio/internal/sqltest",
	"github.com/pingcap/",
	"github.com/pganalyze/",
	"github.com/wasilibs/",
	"github.com/tetratelabs/wazero",
	"modernc.org/",
}

func TestBuildExcludesTestOnlyPackages(t *testing.T) {
	t.Parallel()

	out, err := exec.Command(goTool(t), "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	for _, problem := range testOnlyProblems(string(out)) {
		t.Error(problem)
	}
}

// testOnlyProblems reports every package in a go list -deps listing that
// belongs to testOnly.
func testOnlyProblems(list string) []string {
	var problems []string
	for _, pkg := range strings.Fields(list) {
		for _, prefix := range testOnly {
			if strings.HasPrefix(pkg, prefix) {
				problems = append(problems, "test-only package in the build: "+pkg)
			}
		}
	}

	return problems
}

func TestBinaryContainsNoNetworkCode(t *testing.T) {
	t.Parallel()

	binary := filepath.Join(t.TempDir(), "cuddio")
	build := exec.Command(goTool(t), "build", "-o", binary, ".")
	build.Env = append(build.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(goTool(t), "tool", "nm", binary).Output()
	if err != nil {
		t.Fatalf("go tool nm: %v", err)
	}

	for _, symbol := range networkSymbols(out) {
		t.Errorf("network code linked into the binary: %s", symbol)
	}
}

// networkSymbols returns the symbol names in go tool nm output that match networkSymbol.
func networkSymbols(nm []byte) []string {
	var found []string

	scanner := bufio.NewScanner(bytes.NewReader(nm))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		if name := fields[len(fields)-1]; networkSymbol.MatchString(name) {
			found = append(found, name)
		}
	}

	return found
}

func TestNetworkCheckersCatchProblems(t *testing.T) {
	t.Parallel()

	list := "example.com/a net/http fmt\nnet/http net crypto/tls\nexample.com/b net\ngithub.com/spf13/pflag net\nnet/url\n\n"
	want := []string{
		"network package in the build: net/http",
		"net/http imports net and is not in allowedNetImporters",
		"example.com/b imports net and is not in allowedNetImporters",
	}
	got := networkImportProblems(list)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("networkImportProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !isNetworkPackage("crypto/tls") {
		t.Error("crypto/tls should count as a network package")
	}

	if got := testOnlyProblems("fmt\nmodernc.org/sqlite\ngithub.com/phoenix1331/cuddio/internal/cli\n"); strings.Join(got, ",") != "test-only package in the build: modernc.org/sqlite" {
		t.Errorf("testOnlyProblems() = %q", got)
	}

	nm := []byte("  4a1b20 T net.(*Dialer).DialContext\n  4a1c00 T net.ParseIP\n  4a1d00 T net/http.Get\nU\n")
	if got := networkSymbols(nm); strings.Join(got, ",") != "net.(*Dialer).DialContext,net/http.Get" {
		t.Errorf("networkSymbols() = %q", got)
	}
}
