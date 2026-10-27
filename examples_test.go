package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/cli"
	"github.com/phoenix1331/cuddio/internal/rules"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

// exampleSecrets are the fake values in examples/ that cleaning must hide.
// The AWS documentation keys are left out: cuddio allowlists them.
var exampleSecrets = map[string][]string{
	"laravel.log":        {"alice.jones@example.com", "bob.smith@example.com", "carys.llyr@example.com", "198.51.100.24", "203.0.113.40", "07700 900123", "4242 4242 4242 4242", "9f8Kq2LmXz7Rt4VbN1cWp6Hs3Jd5Ya0E", "Lq7Vn2Rt8Kw3Zx5Pm9Hc4Jd6", "Sm7pPassw0rd2026", "Hs3Jd5Ya0E9f8Kq2LmXz7Rt4", "/home/deploy"},
	"nginx-access.log":   {"198.51.100.24", "203.0.113.77", "192.0.2.10", "2001:db8:85a3::8a2e:370:7334", "Zx81Lm2Qp9Rt7Vb3Nc6Kd4Hs", "alice.jones%40example.com", "bob.smith@example.com", "Kd4Hs3Jd5Ya0E9f8Kq2LmXz7"},
	".env.example-leaky": {"Zm9yIGRlbW9uc3RyYXRpb24", "Pr0d-Db-Passw0rd-2026", "old-password-from-last-year", "r3dis-s3cret-Kq2Lm", "Sm7pPassw0rd2026", "mailer@example.com", "Lq7Vn2Rt8Kw3Zx5Pm9Hc4Jd6Yb1Tf0Gs", "Wx5Pm9Hc4Jd6Lq7Vn2Rt8Kw3"},
	"stacktrace.txt":     {"Pg-Passw0rd-2026", "R3ports-Passw0rd", "Etl-Passw0rd-2026", "/home/alice", "/Users/bob", `C:\Users\Jane Smith`, "jane.smith@example.com", "203.0.113.40", "198.51.100.9"},
	"mysql-dump.sql":     {"alice@example.com", "bob@example.com", "carys@example.com", "Brien", "$2y$12$", "07700 900123", "198.51.100.23", "203.0.113.9"},
	"postgres-dump.sql":  {"alice@example.com", "bob@example.com", "carys@example.com", "Brien", "$2y$12$", "07700 900123", "198.51.100.23", "203.0.113.9"},
	"sqlite-dump.sql":    {"alice@example.com", "bob@example.com", "carys@example.com", "Brien", "$2y$12$", "07700 900123", "198.51.100.23", "203.0.113.9"},
}

// cuddio runs the CLI in-process with the real rules, an empty working
// directory and its own cache, so no config or mapping on the machine
// running the tests can change the result.
func cuddio(t *testing.T, cache string, args ...string) (string, string, int) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Execute(cli.Env{
		Version:  "test",
		Args:     args,
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   &stderr,
		CacheDir: cache,
		Rules:    rules.All(),
	})

	return stdout.String(), stderr.String(), code
}

func TestExamplesClean(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "README.md" || name == "ai-answer.md" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			secrets, ok := exampleSecrets[name]
			if !ok {
				t.Fatalf("no fake values listed for examples/%s; add them to exampleSecrets", name)
			}
			path := filepath.Join("examples", name)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range secrets {
				if !strings.Contains(string(original), s) {
					t.Fatalf("examples/%s no longer holds %q; update exampleSecrets", name, s)
				}
			}

			out, stderr, code := cuddio(t, t.TempDir(), path)
			if code != 0 || stderr != "" {
				t.Fatalf("cuddio %s = %d %q", name, code, stderr)
			}
			for _, s := range secrets {
				if strings.Contains(out, s) {
					t.Errorf("%q survived cleaning", s)
				}
			}
			if d, ok := sqltest.DialectOf(strings.TrimSuffix(name, "-dump.sql") + ".sql"); ok && strings.HasSuffix(name, ".sql") {
				if err := sqltest.Check(d, out); err != nil {
					t.Errorf("cleaned %s is not valid %s: %v", name, d, err)
				}
			}
		})
	}
}

func TestExampleAIAnswerMatchesTheLog(t *testing.T) {
	t.Parallel()

	answer, err := os.ReadFile(filepath.Join("examples", "ai-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	cleaned, stderr, code := cuddio(t, cache, filepath.Join("examples", "laravel.log"), "--map")
	if code != 0 {
		t.Fatalf("cuddio laravel.log --map = %d %q", code, stderr)
	}

	// Every placeholder in the answer comes from the cleaned log, except
	// <EMAIL_9>, which shows restore leaving unknown ones alone
	for _, token := range regexp.MustCompile(`<[A-Z_]+_[0-9]+>`).FindAllString(string(answer), -1) {
		if token != "<EMAIL_9>" && !strings.Contains(cleaned, token) {
			t.Errorf("ai-answer.md uses %s, which cleaning laravel.log does not produce", token)
		}
	}

	restored, stderr, code := cuddio(t, cache, "restore", filepath.Join("examples", "ai-answer.md"))
	if code != 0 {
		t.Fatalf("cuddio restore = %d %q", code, stderr)
	}
	for _, want := range []string{"alice.jones@example.com", "Sm7pPassw0rd2026", "4242 4242 4242 4242", "<EMAIL_9>"} {
		if !strings.Contains(restored, want) {
			t.Errorf("restored answer lacks %q", want)
		}
	}
}
