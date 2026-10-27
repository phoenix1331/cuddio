package cli

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/phoenix1331/cuddio/internal/config"
	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/textscan"
)

const (
	formatText = "text"
	formatSQL  = "sql"
)

// cleanFlags are the root command's flags for cleaning input.
type cleanFlags struct {
	output  string
	force   bool
	format  string
	dialect string
}

// clean reads the file in args, or stdin, and writes the redacted text to
// stdout or --output.
func (a *app) clean(args []string, flags cleanFlags) error {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	format, err := resolveFormat(flags.format, flags.dialect, path)
	if err != nil {
		return err
	}
	var dialect sqlscan.Dialect
	if flags.dialect != "" {
		d, ok := sqlscan.ParseDialect(flags.dialect)
		if !ok {
			return errors.New("invalid --dialect: want mysql, postgres or sqlite")
		}
		dialect = d
	}
	if a.config.Mode == config.ModeFake {
		return errors.New("fake mode is not supported yet")
	}

	redactor, err := a.redactor()
	if err != nil {
		return err
	}

	in, inInfo, err := a.openInput(path)
	if err != nil {
		return err
	}
	defer in.Close()

	run, err := a.cleaner(format, dialect, in, redactor)
	if err != nil {
		return err
	}
	if flags.output == "" {
		return run(a.env.Stdout)
	}

	return writeOutput(flags.output, flags.force, inInfo, run)
}

// cleaner returns the function that cleans in for format. For SQL without
// a dialect, the dialect is detected from the start of the input, which is
// buffered rather than consumed.
func (a *app) cleaner(format string, dialect sqlscan.Dialect, in io.Reader, rd *detect.Redactor) (func(io.Writer) error, error) {
	if format == formatText {
		return func(w io.Writer) error {
			return textscan.Clean(w, in, rd.Redact, textscan.Options{})
		}, nil
	}

	buffered := bufio.NewReaderSize(in, sqlscan.DetectSize)
	if dialect == "" {
		// A read error here comes back again when cleaning reads on
		header, _ := buffered.Peek(sqlscan.DetectSize)
		detected, ok := sqlscan.Detect(header)
		if !ok && len(header) > 0 {
			return nil, errors.New("cannot tell the SQL dialect from the start of the input; use --dialect mysql, postgres or sqlite")
		}
		// Empty input has nothing to clean in any dialect
		dialect = cmp.Or(detected, sqlscan.MySQL)
	}
	cols := sqlscan.NewColumns(a.config.SQL.Columns, a.config.SQL.Tables)

	return func(w io.Writer) error {
		return sqlscan.Clean(w, buffered, dialect, rd, cols, sqlscan.Options{})
	}, nil
}

// redactor builds the run's redactor from the rules and config.
func (a *app) redactor() (*detect.Redactor, error) {
	custom, err := config.CustomRules(a.config.Rules.Custom)
	if err != nil {
		return nil, err
	}
	registry, err := detect.NewRegistry(detect.WithCustom(a.env.Rules, custom))
	if err != nil {
		return nil, err
	}
	selected, err := registry.Select(a.config.Rules.Enable, a.config.Rules.Disable)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	allow, err := detect.NewAllowlist(a.config.Allowlist.Values, a.config.Allowlist.Patterns)
	if err != nil {
		return nil, err
	}
	placeholders, err := replace.NewPlaceholders(a.config.Placeholder.Format)
	if err != nil {
		return nil, err
	}

	return detect.NewRedactor(detect.NewDetector(selected, allow), placeholders), nil
}

// openInput opens path, or stdin when path is empty. The returned file info
// is nil when the input is not a file cuddio can stat.
func (a *app) openInput(path string) (io.ReadCloser, fs.FileInfo, error) {
	if path == "" {
		info, _ := stat(a.env.Stdin)
		return io.NopCloser(a.env.Stdin), info, nil
	}

	f, err := os.Open(path) //nolint:gosec // G304: reading the file the user named is the point
	if err != nil {
		return nil, nil, err
	}
	// Closing a file only read from cannot lose data, so its error is ignored
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s is a directory", path)
	}

	return f, info, nil
}

// writeOutput runs write against a temporary file next to path, then renames
// it into place, so a failed run leaves no partial output and the input is
// never truncated, even when path is a link to it. path must not be the
// input, and must not exist unless force is set.
func writeOutput(path string, force bool, input fs.FileInfo, write func(io.Writer) error) error {
	existing, err := os.Stat(path)
	if err == nil {
		if input != nil && os.SameFile(existing, input) {
			return fmt.Errorf("output %s is the input file", path)
		}
		if !force {
			return fmt.Errorf("output %s exists; use --force to replace it", path)
		}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".cuddio-*")
	if err != nil {
		return fmt.Errorf("output: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	if err := write(tmp); err != nil {
		_ = tmp.Close() // the write error matters more, and the file is removed
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("output: %w", err)
	}

	return nil
}

// resolveFormat returns the --format value, or the format implied by
// --dialect or the file extension (.sql is SQL, anything else text).
func resolveFormat(flag, dialect, path string) (string, error) {
	switch flag {
	case formatText:
		if dialect != "" {
			return "", errors.New("--dialect only applies to --format sql")
		}
		return flag, nil
	case formatSQL:
		return flag, nil
	case "":
		if dialect != "" || strings.EqualFold(filepath.Ext(path), ".sql") {
			return formatSQL, nil
		}
		return formatText, nil
	}

	return "", fmt.Errorf("invalid --format %q: want %s or %s", flag, formatText, formatSQL)
}

// stat returns file info for r when it is a file, such as redirected stdin.
func stat(r io.Reader) (fs.FileInfo, bool) {
	f, ok := r.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return nil, false
	}
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}

	return info, true
}

// isTerminal reports whether r is an interactive terminal rather than a pipe
// or a redirected file.
func isTerminal(r io.Reader) bool {
	info, ok := stat(r)
	return ok && info.Mode()&fs.ModeCharDevice != 0
}
