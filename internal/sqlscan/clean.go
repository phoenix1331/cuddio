package sqlscan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Redactor is what cleaning needs from the rule engine.
type Redactor interface {
	// Redact replaces whatever pattern rules find in s.
	Redact(s string) string
	// Whole replaces all of value, from a sensitive column, with a value
	// labelled label. It may return value as it is, such as when it is
	// allowlisted.
	Whole(label, value string) string
}

// CleanMySQL copies a MySQL or MariaDB dump from r to w. Every string value
// and comment goes through rd.Redact, and every literal in a sensitive
// column of an INSERT row is replaced whole through rd.Whole, so a name or
// password hash is hidden even when no pattern matches it. Columns come from
// the INSERT column list, or from the table's CREATE TABLE earlier in the
// dump.
//
// A changed value is written as a quoted string, escaped as mysqldump
// escapes strings; everything else is copied byte for byte.
func CleanMySQL(w io.Writer, r io.Reader, rd Redactor, cols *Columns, opts Options) error {
	return clean(w, NewMySQL(r, opts), rd, cols)
}

// CleanPostgres copies a pg_dump file from r to w, as CleanMySQL does for
// MySQL. COPY ... FROM stdin rows are cleaned field by field, with columns
// from the COPY column list; a COPY with options (such as CSV) has each row
// checked by pattern rules as a whole. psql meta-commands go through
// pattern rules too, except \restrict and \unrestrict, whose key psql checks.
//
// A changed value is written as a standard string ('...', quotes doubled),
// or as an E'...' string when it was one; a changed COPY field is escaped as
// pg_dump escapes it.
func CleanPostgres(w io.Writer, r io.Reader, rd Redactor, cols *Columns, opts Options) error {
	return clean(w, NewPostgres(r, opts), rd, cols)
}

func clean(w io.Writer, tz *Tokenizer, rd Redactor, cols *Columns) error {
	out := bufio.NewWriter(w)
	st := newStatement(cols)

	for {
		tok, err := tz.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		text := tok.Text
		switch tok.Kind {
		case Whitespace, DirectiveStart, DirectiveEnd:
		case Comment:
			text = cleanComment(tok, rd)
		case Meta:
			text = cleanMeta(tok, rd)
		case CopyData:
			text = cleanCopyRow(tok.Text, rd, st.copy)
		case CopyEnd:
			st.copy = nil
		default:
			text = cleanValue(tok, rd, st.label())
			st.observe(tok)
		}
		if _, err := out.WriteString(text); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}

	if err := out.Flush(); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// cleanValue returns tok's text with its value redacted. label is set when
// tok is a value in a sensitive column, which is replaced whole.
func cleanValue(tok Token, rd Redactor, label string) string {
	switch tok.Kind {
	case String:
		value := tok.Value()
		cleaned := rd.Redact(value)
		if label != "" {
			cleaned = rd.Whole(label, value)
		}
		if cleaned == value {
			return tok.Text
		}
		return quote(tok, cleaned)
	case Number, Hex, Bit:
		if label == "" {
			return tok.Text
		}
		if cleaned := rd.Whole(label, tok.Text); cleaned != tok.Text {
			return quote(tok, cleaned)
		}
	}

	return tok.Text
}

var (
	// mysqlEscaper escapes a string value the way mysqldump does.
	mysqlEscaper = strings.NewReplacer("\\", `\\`, "'", `\'`, "\"", `\"`, "\n", `\n`, "\r", `\r`, "\x00", `\0`, "\x1a", `\Z`)
	// escapeE escapes a PostgreSQL E'...' string.
	escapeE = strings.NewReplacer("\\", `\\`, "'", "''")
	// escapeStandard escapes a PostgreSQL standard string.
	escapeStandard = strings.NewReplacer("'", "''")
)

// quote writes s as a string literal in place of tok. A MySQL String keeps
// its quote character and prefix; a PostgreSQL one keeps an N or E prefix,
// and a dollar-quoted one becomes a standard string. Other literals become
// single-quoted strings.
func quote(tok Token, s string) string {
	prefix, q := "", "'"
	if tok.Kind == String && tok.Text[0] != '$' {
		i := strings.IndexAny(tok.Text, `'"`)
		prefix, q = tok.Text[:i], tok.Text[i:i+1]
	}

	switch {
	case tok.dialect == mysql:
		return prefix + q + mysqlEscaper.Replace(s) + q
	case strings.EqualFold(prefix, "E"):
		return prefix + q + escapeE.Replace(s) + q
	}

	return prefix + q + escapeStandard.Replace(s) + q
}

// cleanComment redacts a comment's body, never its markers, so a match
// cannot run into "--" or "/*". If a placeholder would change where the
// comment ends (a line break in a line comment; "/*" or "*/" in a block
// comment, which PostgreSQL nests) or what it is ("/*!" is a MySQL
// directive), the comment is emptied instead, so its contents can never
// become SQL.
func cleanComment(tok Token, rd Redactor) string {
	open, body, end := commentParts(tok)
	cleaned := rd.Redact(body)
	if cleaned == body {
		return tok.Text
	}

	out := open + cleaned + end
	if open == "/*" {
		if !strings.HasPrefix(cleaned, "!") && sameCount(out, tok.Text, "/*") && sameCount(out, tok.Text, "*/") {
			return out
		}
		return "/**/"
	}
	if strings.ContainsAny(cleaned, "\r\n") {
		return "/**/"
	}

	return out
}

// commentParts splits a comment into its opening marker, body and closing
// marker. A MySQL "--" comment's marker includes the space after it, which
// MySQL needs to see a comment at all.
func commentParts(tok Token) (open, body, end string) {
	text := tok.Text
	switch {
	case strings.HasPrefix(text, "/*"):
		open, body = "/*", text[2:]
		if !tok.Unterminated {
			body, end = body[:len(body)-2], "*/"
		}
		return open, body, end
	case strings.HasPrefix(text, "#"):
		return "#", text[1:], ""
	}

	n := 2
	if tok.dialect == mysql && len(text) > 2 {
		n = 3
	}

	return text[:n], text[n:], ""
}

func sameCount(a, b, sub string) bool {
	return strings.Count(a, sub) == strings.Count(b, sub)
}

// cleanMeta redacts the arguments of a psql meta-command line, never the
// command. \restrict and \unrestrict are kept: their key is random, and
// psql refuses anything but letters and digits. Arguments a placeholder
// would split over lines are dropped.
func cleanMeta(tok Token, rd Redactor) string {
	name, args, ok := strings.Cut(tok.Text, " ")
	if !ok || name == `\restrict` || name == `\unrestrict` {
		return tok.Text
	}
	cleaned := rd.Redact(args)
	if strings.ContainsAny(cleaned, "\r\n") {
		return name
	}

	return name + " " + cleaned
}

// copyEscaper escapes a COPY text-format field the way pg_dump does.
var copyEscaper = strings.NewReplacer("\\", `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`, "\b", `\b`, "\f", `\f`, "\v", `\v`)

// cleanCopyRow redacts one line of COPY data. Fields are split on tabs and
// decoded; \N (NULL) is left alone; sensitive columns are replaced whole and
// other fields checked by pattern rules. Unchanged fields keep their bytes.
// Without text-format columns (rows is nil or raw), the line is checked by
// pattern rules as a whole.
func cleanCopyRow(line string, rd Redactor, rows *copyRows) string {
	body, ending := splitLineEnding(line)
	if rows == nil || rows.raw {
		cleaned := rd.Redact(body)
		if strings.ContainsAny(cleaned, "\r\n") {
			cleaned = strings.NewReplacer("\r", " ", "\n", " ").Replace(cleaned)
		}
		return cleaned + ending
	}

	fields := splitCopyFields(body)
	changed := false
	for i, field := range fields {
		if field == `\N` {
			continue
		}
		value := decodeCopy(field)
		cleaned := rd.Redact(value)
		if i < len(rows.labels) && rows.labels[i] != "" {
			cleaned = rd.Whole(rows.labels[i], value)
		}
		if cleaned != value {
			fields[i] = copyEscaper.Replace(cleaned)
			changed = true
		}
	}
	if !changed {
		return line
	}

	return strings.Join(fields, "\t") + ending
}

// splitCopyFields splits a COPY text row on tabs that no backslash escapes.
func splitCopyFields(row string) []string {
	var fields []string
	start := 0
	for i := 0; i < len(row); i++ {
		switch row[i] {
		case '\\':
			i++
		case '\t':
			fields = append(fields, row[start:i])
			start = i + 1
		}
	}

	return append(fields, row[start:])
}

// decodeCopy decodes a COPY text-format field: \b \f \n \r \t \v, octal
// (\101) and hex (\x41) escapes; any other escaped byte stands for itself.
func decodeCopy(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}

	var b strings.Builder
	b.Grow(len(field))
	for i := 0; i < len(field); i++ {
		if field[i] != '\\' || i+1 == len(field) {
			b.WriteByte(field[i])
			continue
		}
		i += decodeCopyEscape(&b, field[i+1:])
	}

	return b.String()
}

// decodeCopyEscape writes the byte a COPY escape stands for, given the text
// after its backslash, and returns how many bytes of rest it used.
func decodeCopyEscape(b *strings.Builder, rest string) int {
	if rest[0] == 'v' {
		b.WriteByte('\v')
		return 1
	}
	if rest[0] == 'u' || rest[0] == 'U' {
		// \u is not an escape in COPY; it stands for "u"
		b.WriteByte(rest[0])
		return 1
	}

	return decodeE(b, rest)
}
