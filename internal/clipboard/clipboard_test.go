package clipboard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestIsWSL(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{"wsl1": true, "wsl2": true, "ubuntu": false, "azure": false} {
		data, err := os.ReadFile(filepath.Join("testdata", "proc-version", name))
		if err != nil {
			t.Fatal(err)
		}
		if got := IsWSL(string(data)); got != want {
			t.Errorf("IsWSL(%s) = %v, want %v", name, got, want)
		}
	}
	if IsWSL("") {
		t.Error(`IsWSL("") = true`)
	}
}

const (
	wsl2Version   = "Linux version 5.15.153.1-microsoft-standard-WSL2"
	ubuntuVersion = "Linux version 6.8.0-45-generic"
)

// host describes a machine with the given tools installed and environment.
func host(goos, version string, env map[string]string, tools ...string) Host {
	return Host{
		GOOS:        goos,
		ProcVersion: version,
		Getenv:      func(k string) string { return env[k] },
		LookPath: func(name string) (string, error) {
			for _, tool := range tools {
				if tool == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
	}
}

func TestNewChoosesTools(t *testing.T) {
	t.Parallel()

	wayland := map[string]string{"WAYLAND_DISPLAY": "wayland-0"}
	tests := []struct {
		name               string
		host               Host
		read, write, priv  string
		unavailableMessage string
	}{
		{"windows", host("windows", "", nil), powershell, powershell, "private", ""},
		{"wsl prefers windows", host("linux", wsl2Version, wayland, powershell, "wl-copy", "wl-paste", "xclip"), powershell, powershell, "private", ""},
		{"macos", host("darwin", "", nil), "pbpaste", "pbcopy", "pbcopy", ""},
		{"wayland", host("linux", ubuntuVersion, wayland, "wl-copy", "wl-paste", "xclip"), "wl-paste", "wl-copy", "wl-copy", ""},
		{"wayland without wl-paste", host("linux", ubuntuVersion, wayland, "wl-copy", "xclip"), "xclip", "xclip", "xclip", ""},
		{"x11 xclip", host("linux", ubuntuVersion, nil, "xclip", "xsel"), "xclip", "xclip", "xclip", ""},
		{"x11 xsel", host("freebsd", "", nil, "xsel"), "xsel", "xsel", "xsel", ""},
		{"wsl without interop", host("linux", wsl2Version, wayland, "wl-copy", "wl-paste"), "wl-paste", "wl-copy", "wl-copy", ""},
		{"wsl with nothing", host("linux", wsl2Version, nil), "", "", "", "Windows interop"},
		{"linux with nothing", host("linux", ubuntuVersion, nil), "", "", "", "install wl-clipboard (Wayland), or xclip or xsel (X11)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := New(tt.host, nil)
			if tt.unavailableMessage != "" {
				if s.unavailable == nil || !strings.Contains(s.unavailable.Error(), tt.unavailableMessage) {
					t.Errorf("unavailable = %v, want it to mention %q", s.unavailable, tt.unavailableMessage)
				}
				return
			}
			if s.unavailable != nil {
				t.Fatalf("unavailable = %v", s.unavailable)
			}
			priv := s.writePrivate.Name
			if reflect.DeepEqual(s.writePrivate, powershellWritePrivate) {
				priv = "private"
			}
			if s.read.Name != tt.read || s.write.Name != tt.write || priv != tt.priv {
				t.Errorf("tools = %s, %s, %s, want %s, %s, %s", s.read.Name, s.write.Name, priv, tt.read, tt.write, tt.priv)
			}
		})
	}
}

// call is one recorded run of a clipboard tool.
type call struct {
	cmd     Command
	stdin   string
	capture bool
	timed   bool
}

// fakeRunner records calls and answers with stdout, stderr and err.
type fakeRunner struct {
	calls          []call
	stdout, stderr string
	err            error
}

func (f *fakeRunner) run(ctx context.Context, c Command, stdin string, capture bool) (string, string, error) {
	_, timed := ctx.Deadline()
	f.calls = append(f.calls, call{cmd: c, stdin: stdin, capture: capture, timed: timed})

	return f.stdout, f.stderr, f.err
}

func TestRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		host    Host
		runner  fakeRunner
		want    string
		wantErr string
	}{
		{"text", host("darwin", "", nil), fakeRunner{stdout: "Llŷr\r\n"}, "Llŷr\r\n", ""},
		{"empty xclip", host("linux", "", nil, "xclip"), fakeRunner{stderr: "Error: target STRING not available", err: errors.New("exit status 1")}, "", ""},
		{"empty wl-paste", host("linux", "", map[string]string{"WAYLAND_DISPLAY": "w"}, "wl-copy", "wl-paste"), fakeRunner{stderr: "No selection\n", err: errors.New("exit status 1")}, "", ""},
		{"tool fails", host("linux", "", nil, "xclip"), fakeRunner{stderr: "Error: Can't open display: alice@shop.test", err: errors.New("exit status 1")}, "", "clipboard: xclip: exit status 1"},
		{"no tool", host("linux", "", nil), fakeRunner{}, "", "no clipboard tool found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := tt.runner
			got, err := New(tt.host, r.run).Read()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || strings.Contains(err.Error(), "alice") {
					t.Errorf("Read() error = %v, want %q without the tool's stderr", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Read() = %q, %v, want %q", got, err, tt.want)
			}
			if len(r.calls) != 1 || !r.calls[0].capture || !r.calls[0].timed || r.calls[0].stdin != "" {
				t.Errorf("calls = %+v, want one captured, timed run with no input", r.calls)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run("plain and private on windows", func(t *testing.T) {
		t.Parallel()

		r := &fakeRunner{}
		s := New(host("windows", "", nil), r.run)
		if err := s.Write("<EMAIL_1>\r\n", false); err != nil {
			t.Fatal(err)
		}
		if err := s.Write("alice@shop.test", true); err != nil {
			t.Fatal(err)
		}
		if len(r.calls) != 2 || !reflect.DeepEqual(r.calls[0].cmd, powershellWrite) || !reflect.DeepEqual(r.calls[1].cmd, powershellWritePrivate) {
			t.Fatalf("calls = %+v", r.calls)
		}
		for _, c := range r.calls {
			if c.capture || !c.timed {
				t.Errorf("write %+v captured output or had no timeout", c)
			}
			if strings.Contains(strings.Join(c.cmd.Args, " "), c.stdin) {
				t.Error("clipboard text appears in the command's arguments")
			}
		}
		if r.calls[0].stdin != "<EMAIL_1>\r\n" || r.calls[1].stdin != "alice@shop.test" {
			t.Errorf("stdin = %q, %q", r.calls[0].stdin, r.calls[1].stdin)
		}
	})

	t.Run("private falls back to a plain write", func(t *testing.T) {
		t.Parallel()

		r := &fakeRunner{}
		if err := New(host("darwin", "", nil), r.run).Write("x", true); err != nil || len(r.calls) != 1 || r.calls[0].cmd.Name != "pbcopy" {
			t.Errorf("Write() = %v, calls %+v", err, r.calls)
		}
		if env := r.calls[0].cmd.Env; len(env) != 1 || env[0] != "LANG=en_US.UTF-8" {
			t.Errorf("pbcopy env = %v, want a UTF-8 locale", env)
		}
	})

	t.Run("empty text is not written", func(t *testing.T) {
		t.Parallel()

		r := &fakeRunner{}
		if err := New(host("windows", "", nil), r.run).Write("", false); err != nil || len(r.calls) != 0 {
			t.Errorf("Write(\"\") = %v, calls %d", err, len(r.calls))
		}
	})

	t.Run("tool fails", func(t *testing.T) {
		t.Parallel()

		r := &fakeRunner{err: errors.New("exit status 1")}
		err := New(host("linux", "", nil, "xsel"), r.run).Write("alice@shop.test", false)
		if err == nil || err.Error() != "clipboard: xsel: exit status 1" {
			t.Errorf("Write() = %v", err)
		}
	})

	t.Run("no tool", func(t *testing.T) {
		t.Parallel()

		if err := New(host("linux", "", nil), nil).Write("x", false); err == nil {
			t.Error("Write() with no tool succeeded")
		}
	})
}

func TestHostFromOS(t *testing.T) {
	t.Parallel()

	h := HostFromOS()
	if h.GOOS != runtime.GOOS || h.Getenv == nil || h.LookPath == nil {
		t.Errorf("HostFromOS() = %+v", h)
	}
}

// helper returns a Command that runs this test binary as a stand-in
// clipboard tool doing mode. os.Executable, not os.Args[0], which can be a
// bare name that Windows will not look up in the current directory.
func helper(t *testing.T, mode string) Command {
	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	return Command{Name: self, Args: []string{"-test.run=^TestHelperProcess$", "--", mode}, Env: []string{"CUDDIO_CLIPBOARD_HELPER=1", "LANG=en_US.UTF-8"}}
}

// TestHelperProcess is not a test: Exec tests run the test binary with it
// as a stand-in clipboard tool.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("CUDDIO_CLIPBOARD_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	in, _ := io.ReadAll(os.Stdin)
	switch mode {
	case "echo":
		fmt.Fprint(os.Stdout, string(in)+"|"+os.Getenv("LANG"))
	case "fail":
		fmt.Fprint(os.Stderr, "target STRING not available")
		os.Exit(1)
	case "linger":
		// Like xclip: output goes to an inherited stdout, then it lingers
		fmt.Fprint(os.Stdout, "serving")
	}
	os.Exit(0)
}

func TestExec(t *testing.T) {
	t.Parallel()

	out, stderr, err := Exec(context.Background(), helper(t, "echo"), "Llŷr ✓\r\n", true)
	if err != nil || out != "Llŷr ✓\r\n|en_US.UTF-8" || stderr != "" {
		t.Errorf("Exec(echo) = %q, %q, %v", out, stderr, err)
	}

	out, stderr, err = Exec(context.Background(), helper(t, "fail"), "", true)
	if err == nil || out != "" || stderr != "target STRING not available" {
		t.Errorf("Exec(fail) = %q, %q, %v", out, stderr, err)
	}

	out, _, err = Exec(context.Background(), helper(t, "linger"), "x", false)
	if err != nil || out != "" {
		t.Errorf("Exec(linger) without capture = %q, %v", out, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Exec(ctx, helper(t, "echo"), "", true); err == nil {
		t.Error("Exec() with a cancelled context succeeded")
	}
}

// TestRealClipboard round-trips text through the machine's real clipboard.
// It overwrites the clipboard, so it only runs with CUDDIO_CLIPBOARD_TEST=1,
// and puts the old contents back afterwards.
func TestRealClipboard(t *testing.T) {
	if os.Getenv("CUDDIO_CLIPBOARD_TEST") != "1" {
		t.Skip("set CUDDIO_CLIPBOARD_TEST=1 to use the real clipboard")
	}

	s := New(HostFromOS(), Exec)
	saved, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Write(saved, false); err != nil {
			t.Errorf("putting the clipboard back: %v", err)
		}
	})

	for _, text := range []string{"Llŷr café ✓ 😀 <EMAIL_1>\nline two\n", "a\r\nb\r\n", "no final newline", strings.Repeat("x", 1<<20)} {
		for _, private := range []bool{false, true} {
			if err := s.Write(text, private); err != nil {
				t.Fatal(err)
			}
			got, err := s.Read()
			if err != nil || got != text {
				t.Errorf("round trip (private %v) of %.20q = %.20q (%d bytes), %v", private, text, got, len(got), err)
			}
		}
	}
}
