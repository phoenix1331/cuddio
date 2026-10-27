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
	"github.com/phoenix1331/cuddio/internal/mapstore"
	"github.com/phoenix1331/cuddio/internal/replace"
	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/textscan"
)

const (
	formatText = "text"
	formatSQL  = "sql"
)

// ioFlags are the flags for what to read and where to write, shared by the
// root command and restore.
type ioFlags struct {
	output  string
	force   bool
	format  string
	dialect string
}

// cleanFlags are the root command's flags for cleaning input.
type cleanFlags struct {
	ioFlags
	mode    string
	mapping bool
}

// job is one run over an input: the file (empty for stdin), how to read it
// and where to write the result.
type job struct {
	path    string
	format  string
	dialect sqlscan.Dialect
	flags   ioFlags
}

// newJob checks the flags for args. Nothing is opened yet.
func newJob(args []string, flags ioFlags) (job, error) {
	j := job{flags: flags}
	if len(args) > 0 {
		j.path = args[0]
	}
	format, err := resolveFormat(flags.format, flags.dialect, j.path)
	if err != nil {
		return job{}, err
	}
	j.format = format
	if flags.dialect != "" {
		d, ok := sqlscan.ParseDialect(flags.dialect)
		if !ok {
			return job{}, errors.New("invalid --dialect: want mysql, postgres or sqlite")
		}
		j.dialect = d
	}

	return j, nil
}

// errAmbiguous is returned after cleaning with --map when the input held a
// placeholder that the session also used for a real value.
var errAmbiguous = errors.New("the input already contains placeholders that this session uses for other values, so restore could not tell them apart; the output was written but the mapping was not stored (run cuddio forget, or clean the original input instead)")

// clean reads the file in args, or stdin, and writes the redacted text to
// stdout or --output. With --map, the placeholders carry on from the stored
// session mapping, and the mapping is saved once the output is written.
func (a *app) clean(args []string, flags cleanFlags) error {
	j, err := newJob(args, flags.ioFlags)
	if err != nil {
		return err
	}
	mode := cmp.Or(flags.mode, a.config.Mode)
	if mode != config.ModePlaceholder && mode != config.ModeFake {
		return errors.New("invalid --mode: want placeholder or fake")
	}
	if flags.mapping && mode == config.ModeFake {
		return errors.New("--map needs placeholder mode: fake values cannot be restored")
	}

	detector, err := a.detector()
	if err != nil {
		return err
	}
	if mode == config.ModeFake {
		return a.run(j, detect.NewRedactor(detector, replace.NewFakes()))
	}
	placeholders, err := replace.NewPlaceholders(a.config.Placeholder.Format)
	if err != nil {
		return err
	}
	if !flags.mapping {
		return a.run(j, detect.NewRedactor(detector, placeholders))
	}

	store, err := a.mappingStore()
	if err != nil {
		return err
	}
	if a.config.Mapping.Disabled {
		return fmt.Errorf("--map: %w", mapstore.ErrDisabled)
	}
	pairs, err := store.Load()
	if errors.Is(err, mapstore.ErrCorrupt) {
		fmt.Fprintln(a.env.Stderr, "cuddio: warning: the stored mapping was unreadable and has been deleted; starting a new one")
		err = nil
	}
	if err != nil {
		return err
	}
	placeholders.Seed(pairs)

	if err := a.run(j, detect.NewRedactor(detector, placeholders)); err != nil {
		return err
	}
	if placeholders.Ambiguous() {
		return errAmbiguous
	}

	return store.Save(placeholders.Pairs())
}

// run opens the job's input, passes it through rd as text or SQL, and
// writes the result to stdout or --output.
func (a *app) run(j job, rd sqlscan.Redactor) error {
	in, inInfo, err := a.openInput(j.path)
	if err != nil {
		return err
	}
	defer in.Close()

	write, err := a.writer(j.format, j.dialect, in, rd)
	if err != nil {
		return err
	}
	if j.flags.output == "" {
		return write(a.env.Stdout)
	}

	return writeOutput(j.flags.output, j.flags.force, inInfo, write)
}

// writer returns the function that passes in through rd for format. For
// SQL without a dialect, the dialect is detected from the start of the
// input, which is buffered rather than consumed.
func (a *app) writer(format string, dialect sqlscan.Dialect, in io.Reader, rd sqlscan.Redactor) (func(io.Writer) error, error) {
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

// detector builds the run's rule set and allowlist from the rules and
// config.
func (a *app) detector() (*detect.Detector, error) {
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

	return detect.NewDetector(selected, allow), nil
}

// mappingStore returns the session mapping store, configured from config.
func (a *app) mappingStore() (*mapstore.Store, error) {
	if a.env.CacheDir == "" {
		return nil, errors.New("no user cache directory to keep mappings in")
	}

	return mapstore.New(mapstore.Options{
		Dir:      a.env.CacheDir,
		TTL:      a.config.Mapping.TTL,
		Disabled: a.config.Mapping.Disabled,
		Now:      a.env.Now,
	}), nil
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
