package cli

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

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
	clip    bool
}

// cleanFlags are the root command's flags for cleaning input.
type cleanFlags struct {
	ioFlags
	mode    string
	mapping bool
	check   bool
	report  bool
}

// job is one run over an input: the file (empty for stdin), how to read it
// and where to write the result.
type job struct {
	path    string
	format  string
	dialect sqlscan.Dialect
	flags   ioFlags
	// private keeps text written back to the clipboard out of clipboard
	// history, for restore, whose output holds real values.
	private bool
	// changed and unchanged report a --clip run that wrote nothing else.
	changed, unchanged string
	// discard drops the output, for --check and --report.
	discard bool
}

// newJob checks the flags for args. Nothing is opened yet.
func newJob(args []string, flags ioFlags) (job, error) {
	j := job{flags: flags}
	if len(args) > 0 {
		j.path = args[0]
	}
	if flags.clip && j.path != "" {
		return job{}, errors.New("--clip reads the clipboard, so it takes no FILE")
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
	j.changed, j.unchanged = "clipboard cleaned", "nothing to hide on the clipboard; left as it was"
	mode := cmp.Or(flags.mode, a.config.Mode)
	if mode != config.ModePlaceholder && mode != config.ModeFake {
		return errors.New("invalid --mode: want placeholder or fake")
	}
	if flags.mapping && mode == config.ModeFake {
		return errors.New("--map needs placeholder mode: fake values cannot be restored")
	}
	counting := flags.check || flags.report
	if counting && (j.flags.output != "" || flags.mapping) {
		return errors.New("--check and --report print counts only, so they take no --output or --map")
	}

	detector, err := a.detector()
	if err != nil {
		return err
	}
	if counting {
		j.discard = true
		// Counts do not depend on what replaces each value
		rd := detect.NewRedactor(detector, replace.NewFakes())
		if err := a.run(j, rd); err != nil {
			return err
		}
		return a.printCounts(rd.Counts(), flags.check, flags.report)
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
	if j.flags.clip {
		return a.runClipboard(j, rd)
	}
	file, inInfo, err := a.openInput(j.path)
	if err != nil {
		return err
	}
	defer file.Close()
	in, err := checkEncoding(file)
	if err != nil {
		return err
	}

	write, err := a.writer(j.format, j.dialect, in, rd)
	if err != nil {
		return err
	}
	if j.discard {
		return write(io.Discard)
	}
	if j.flags.output == "" {
		return write(a.env.Stdout)
	}

	return writeOutput(j.flags.output, j.flags.force, inInfo, write)
}

// runClipboard passes the clipboard's text through rd and writes the result
// back to the clipboard, or to --output. A note on stderr says what
// happened, since nothing else is printed.
func (a *app) runClipboard(j job, rd sqlscan.Redactor) error {
	if a.env.Clipboard == nil {
		return errors.New("no clipboard is available")
	}
	text, err := a.env.Clipboard.Read()
	if err != nil {
		return err
	}
	if text == "" {
		return errors.New("the clipboard holds no text")
	}
	write, err := a.writer(j.format, j.dialect, strings.NewReader(text), rd)
	if err != nil {
		return err
	}
	if j.discard {
		return write(io.Discard)
	}
	if j.flags.output != "" {
		return writeOutput(j.flags.output, j.flags.force, nil, write)
	}

	var out strings.Builder
	if err := write(&out); err != nil {
		return err
	}
	if out.String() == text {
		_, err := fmt.Fprintln(a.env.Stderr, "cuddio: "+j.unchanged)
		return err
	}
	if err := a.env.Clipboard.Write(out.String(), j.private); err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.env.Stderr, "cuddio: "+j.changed)

	return err
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

// errUTF16 explains why UTF-16 input is refused.
var errUTF16 = errors.New("the input is UTF-16 text, as Windows PowerShell 5.1 writes with > and Out-File; no rule can match it, so nothing would be hidden. Convert it to UTF-8 first, for example: Get-Content FILE | Set-Content -Encoding utf8 FILE.utf8")

// checkEncoding refuses input that starts with a UTF-16 (or UTF-32) byte
// order mark, which would pass through every rule unredacted, and returns
// a reader that still starts at the first byte.
func checkEncoding(in io.Reader) (io.Reader, error) {
	buffered := bufio.NewReader(in)
	// A read error here comes back again when cleaning reads on
	head, _ := buffered.Peek(2)
	if string(head) == "\xff\xfe" || string(head) == "\xfe\xff" {
		return nil, errUTF16
	}

	return buffered, nil
}

// errFindings is returned by --check when something would be hidden; it
// sets exit code 1 and prints nothing more.
var errFindings = errors.New("values would be hidden")

// printCounts writes --report's table of counts by rule ID and --check's
// total. Only rule IDs, column labels and numbers are printed, never
// values.
func (a *app) printCounts(counts map[string]int, check, report bool) error {
	total := 0
	for _, n := range counts {
		total += n
	}
	if report {
		tw := tabwriter.NewWriter(a.env.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "RULE\tCOUNT")
		for _, id := range slices.Sorted(maps.Keys(counts)) {
			fmt.Fprintf(tw, "%s\t%d\n", id, counts[id])
		}
		fmt.Fprintf(tw, "total\t%d\n", total)
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if !check {
		return nil
	}
	if total == 0 {
		_, err := fmt.Fprintln(a.env.Stdout, "nothing to hide")
		return err
	}
	noun := "values"
	if total == 1 {
		noun = "value"
	}
	if _, err := fmt.Fprintf(a.env.Stdout, "%d %s would be hidden\n", total, noun); err != nil {
		return err
	}

	return errFindings
}

// rules returns every rule cuddio knows (built-in, vendored and custom from
// config) and the ones config turns on, both in registry order.
func (a *app) rules() (all, selected []detect.Rule, err error) {
	custom, err := config.CustomRules(a.config.Rules.Custom)
	if err != nil {
		return nil, nil, err
	}
	registry, err := detect.NewRegistry(detect.WithCustom(a.env.Rules, custom))
	if err != nil {
		return nil, nil, err
	}
	selected, err = registry.Select(a.config.Rules.Enable, a.config.Rules.Disable)
	if err != nil {
		return nil, nil, fmt.Errorf("rules: %w", err)
	}

	return registry.Rules(), selected, nil
}

// detector builds the run's rule set and allowlist from the rules and
// config.
func (a *app) detector() (*detect.Detector, error) {
	_, selected, err := a.rules()
	if err != nil {
		return nil, err
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
