package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWiresVersionAndOutput(t *testing.T) {
	original := version
	version = "v9.9.9"
	t.Cleanup(func() { version = original })

	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if got, want := stdout.String(), "cuddio v9.9.9\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
