package cli

import (
	"bytes"
	"strings"
	"testing"
)

type result struct {
	code   int
	stdout string
	stderr string
}

func execute(t *testing.T, env Env) result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	env.Stdin = strings.NewReader("")
	env.Stdout = &stdout
	env.Stderr = &stderr
	if env.ReadBuildInfo == nil {
		env.ReadBuildInfo = buildInfo("", false)
	}

	code := Execute(env)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestVersionFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  Env
		want string
	}{
		{"stamped version", Env{Version: "v0.1.0", Args: []string{"--version"}}, "cuddio v0.1.0\n"},
		{"build info version", Env{Args: []string{"--version"}, ReadBuildInfo: buildInfo("v0.2.0", true)}, "cuddio v0.2.0\n"},
		{"short flag", Env{Args: []string{"-v"}}, "cuddio dev\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, tt.env)

			if got.code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr %q)", got.code, exitOK, got.stderr)
			}
			if got.stdout != tt.want {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.want)
			}
		})
	}
}

func TestVersionFallsBackToRealBuildInfo(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	code := Execute(Env{Args: []string{"--version"}, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &bytes.Buffer{}})

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.HasPrefix(stdout.String(), "cuddio ") {
		t.Errorf("stdout = %q, want a cuddio version line", stdout.String())
	}
}

func TestHelpIncludesBestEffortNotice(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"--help"}} {
		got := execute(t, Env{Args: args})

		if got.code != exitOK {
			t.Fatalf("args %q: exit code = %d, want %d", args, got.code, exitOK)
		}
		for _, want := range []string{"PII", "never makes a network connection", "Review the output before you share it"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("args %q: help does not contain %q", args, want)
			}
		}
	}
}

func TestUsageErrorsExitWithError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--nope"}, "cuddio: unknown flag: --nope\n"},
		{"unexpected argument", []string{"extra"}, `cuddio: unknown command "extra" for "cuddio"` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Args: tt.args})

			if got.code != exitError {
				t.Errorf("exit code = %d, want %d", got.code, exitError)
			}
			if got.stderr != tt.want {
				t.Errorf("stderr = %q, want %q", got.stderr, tt.want)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing", got.stdout)
			}
		})
	}
}
