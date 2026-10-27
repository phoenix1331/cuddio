// Package clipboard reads and writes the system clipboard as UTF-8 text by
// running the platform's own tools: PowerShell on Windows and WSL, pbcopy
// and pbpaste on macOS, wl-clipboard, xclip or xsel on Linux. Text always
// goes through stdin and stdout, never command-line arguments, and a
// tool's own error output is never shown, since it can echo clipboard text.
package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Clipboard is the system clipboard, as cuddio uses it.
type Clipboard interface {
	// Read returns the clipboard's text, or "" when it holds none.
	Read() (string, error)
	// Write replaces the clipboard's text. private asks the platform to
	// keep it out of clipboard history and cloud sync, where it can; use it
	// for text holding real values. Empty text is not written.
	Write(text string, private bool) error
}

// Host is what choosing clipboard tools needs to know about the machine.
type Host struct {
	// GOOS is runtime.GOOS.
	GOOS string
	// ProcVersion is the contents of /proc/version, or "" without one.
	ProcVersion string
	Getenv      func(string) string
	LookPath    func(string) (string, error)
}

// HostFromOS describes the machine cuddio is running on.
func HostFromOS() Host {
	// No /proc/version just means not Linux, so the error is not needed
	version, _ := os.ReadFile("/proc/version")

	return Host{GOOS: runtime.GOOS, ProcVersion: string(version), Getenv: os.Getenv, LookPath: exec.LookPath}
}

// IsWSL reports whether a /proc/version string comes from Windows
// Subsystem for Linux: "Microsoft" in WSL 1, "microsoft-standard-WSL2" in
// WSL 2.
func IsWSL(procVersion string) bool {
	return strings.Contains(strings.ToLower(procVersion), "microsoft")
}

// Command is one clipboard tool invocation.
type Command struct {
	Name string
	Args []string
	// Env is added to the environment, such as LANG for pbcopy.
	Env []string
	// Empty lists exit-error messages that mean the clipboard holds no
	// text, such as xclip's "target STRING not available".
	Empty []string
}

// Runner runs c with stdin as its input. With capture set, it returns the
// command's stdout and stderr; without it, output is discarded, because
// tools like xclip keep running to serve the clipboard and a captured
// stdout would make the run wait for them.
type Runner func(ctx context.Context, c Command, stdin string, capture bool) (stdout, stderr string, err error)

// Timeout is how long a clipboard tool may take.
const Timeout = 10 * time.Second

// System is the clipboard of the machine described by a Host.
type System struct {
	read, write, writePrivate Command
	// unavailable is set when no tool was found; every call returns it.
	unavailable error
	run         Runner
}

// New picks the clipboard tools for host. When none is installed, the
// System returns a helpful error on use rather than here, so commands that
// never touch the clipboard are unaffected.
func New(host Host, run Runner) *System {
	s := &System{run: run}
	switch {
	case host.GOOS == "windows" || (IsWSL(host.ProcVersion) && found(host, powershell)):
		s.read, s.write, s.writePrivate = powershellRead, powershellWrite, powershellWritePrivate
	case host.GOOS == "darwin":
		s.read, s.write = pbpaste, pbcopy
	case host.Getenv("WAYLAND_DISPLAY") != "" && found(host, "wl-copy") && found(host, "wl-paste"):
		s.read, s.write = wlPaste, wlCopy
	case found(host, "xclip"):
		s.read, s.write = xclipRead, xclipWrite
	case found(host, "xsel"):
		s.read, s.write = xselRead, xselWrite
	case IsWSL(host.ProcVersion):
		s.unavailable = errors.New("no clipboard tool found: powershell.exe cannot be run (is Windows interop turned off in /etc/wsl.conf?), and neither wl-clipboard nor xclip nor xsel is installed")
	default:
		s.unavailable = errors.New("no clipboard tool found: install wl-clipboard (Wayland), or xclip or xsel (X11)")
	}
	if s.writePrivate.Name == "" {
		s.writePrivate = s.write
	}

	return s
}

func found(host Host, name string) bool {
	_, err := host.LookPath(name)
	return err == nil
}

// Read returns the clipboard's text.
func (s *System) Read() (string, error) {
	if s.unavailable != nil {
		return "", s.unavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	out, stderr, err := s.run(ctx, s.read, "", true)
	if err != nil {
		for _, empty := range s.read.Empty {
			if strings.Contains(stderr, empty) {
				return "", nil
			}
		}
		return "", fmt.Errorf("clipboard: %s: %w", s.read.Name, err)
	}

	return out, nil
}

// Write replaces the clipboard's text.
func (s *System) Write(text string, private bool) error {
	if s.unavailable != nil {
		return s.unavailable
	}
	if text == "" {
		return nil
	}
	c := s.write
	if private {
		c = s.writePrivate
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	if _, _, err := s.run(ctx, c, text, false); err != nil {
		return fmt.Errorf("clipboard: %s: %w", c.Name, err)
	}

	return nil
}

// Exec runs clipboard tools as processes.
func Exec(ctx context.Context, c Command, stdin string, capture bool) (string, string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...) //nolint:gosec // G204: the names and arguments are fixed tool invocations from this package
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	if capture {
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
	}
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

const powershell = "powershell.exe"

// PowerShell scripts. Text crosses stdin and stdout as UTF-8; Get-Clipboard
// -Raw keeps line endings exactly, and Out.Write adds no newline. The
// private write marks the text so Windows clipboard history, cloud sync
// and clipboard monitors skip it.
var (
	powershellRead = Command{Name: powershell, Args: []string{"-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8; $c=Get-Clipboard -Raw; if ($c -ne $null) { [Console]::Out.Write($c) }`}}
	powershellWrite = Command{Name: powershell, Args: []string{"-NoProfile", "-NonInteractive", "-Command",
		`[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())`}}
	powershellWritePrivate = Command{Name: powershell, Args: []string{"-NoProfile", "-NonInteractive", "-Command",
		`[Console]::InputEncoding=[Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; ` +
			`$d=New-Object System.Windows.Forms.DataObject; $d.SetText([Console]::In.ReadToEnd(), [System.Windows.Forms.TextDataFormat]::UnicodeText); ` +
			`foreach ($f in 'ExcludeClipboardContentFromMonitorProcessing','CanIncludeInClipboardHistory','CanUploadToCloudClipboard') { $d.SetData($f, (New-Object System.IO.MemoryStream(,[byte[]](0,0,0,0)))) }; ` +
			`[System.Windows.Forms.Clipboard]::SetDataObject($d, $true)`}}
)

// pbcopy and pbpaste read and write in the locale's encoding, so UTF-8 is
// set for them; without it (cron, some launchers) non-ASCII is mangled.
var (
	pbpaste = Command{Name: "pbpaste", Env: []string{"LANG=en_US.UTF-8"}}
	pbcopy  = Command{Name: "pbcopy", Env: []string{"LANG=en_US.UTF-8"}}
)

var (
	wlPaste    = Command{Name: "wl-paste", Args: []string{"--no-newline"}, Empty: []string{"No selection", "Nothing is copied"}}
	wlCopy     = Command{Name: "wl-copy"}
	xclipRead  = Command{Name: "xclip", Args: []string{"-selection", "clipboard", "-out"}, Empty: []string{"target STRING not available", "target UTF8_STRING not available"}}
	xclipWrite = Command{Name: "xclip", Args: []string{"-selection", "clipboard", "-in"}}
	xselRead   = Command{Name: "xsel", Args: []string{"--clipboard", "--output"}}
	xselWrite  = Command{Name: "xsel", Args: []string{"--clipboard", "--input"}}
)
