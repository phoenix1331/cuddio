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
	tz := NewMySQL(r, opts)
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

// stringEscaper escapes a string value the way mysqldump does.
var stringEscaper = strings.NewReplacer("\\", `\\`, "'", `\'`, "\"", `\"`, "\n", `\n`, "\r", `\r`, "\x00", `\0`, "\x1a", `\Z`)

// quote writes s as a string literal in place of tok, keeping a String's
// quote character and N prefix. Other literals become single-quoted strings.
func quote(tok Token, s string) string {
	prefix, q := "", "'"
	if tok.Kind == String {
		i := strings.IndexAny(tok.Text, `'"`)
		prefix, q = tok.Text[:i], tok.Text[i:i+1]
	}

	return prefix + q + stringEscaper.Replace(s) + q
}

// cleanComment redacts a comment's text. If a placeholder would end the
// comment early (a line break in a line comment, "*/" in a block comment),
// the comment is emptied instead, so its contents can never become SQL.
func cleanComment(tok Token, rd Redactor) string {
	cleaned := rd.Redact(tok.Text)
	if cleaned == tok.Text {
		return tok.Text
	}

	if strings.HasPrefix(tok.Text, "/*") {
		inner, ok := strings.CutPrefix(cleaned, "/*")
		if ok && !tok.Unterminated {
			inner, ok = strings.CutSuffix(inner, "*/")
		}
		if ok && !strings.Contains(inner, "*/") {
			return cleaned
		}
		return "/**/"
	}
	if strings.ContainsAny(cleaned, "\r\n") {
		return "/**/"
	}

	return cleaned
}

// statementKind is what the statement being read turned out to be.
type statementKind int

const (
	// atStart: no significant token yet.
	atStart statementKind = iota
	// otherStatement: nothing to track until the next ";".
	otherStatement
	// afterCreate: CREATE seen, TABLE may follow.
	afterCreate
	createTable
	insert
)

// phase is where a CREATE TABLE or INSERT has got to.
type phase int

const (
	// tableName: reading the table name.
	tableName phase = iota
	// afterName: CREATE TABLE expects "(", INSERT a column list or VALUES.
	afterName
	// columnList: inside CREATE TABLE's definitions or INSERT's column list.
	columnList
	// values: INSERT rows.
	values
	// finished: nothing more to track.
	finished
)

// statement follows the statement being cleaned closely enough to know
// which column each INSERT value belongs to. It keeps the column names of
// every table created so far, and nothing else from earlier statements.
type statement struct {
	cols   *Columns
	tables map[string][]string

	kind    statementKind
	phase   phase
	depth   int
	table   string
	columns []string
	// explicit is set once an INSERT has its own column list.
	explicit bool
	// newColumn is set where a CREATE TABLE definition starts, so a name
	// there is a column unless it is a keyword such as PRIMARY.
	newColumn bool
	// labels holds each INSERT column's label, "" for columns that are not
	// sensitive, worked out once per statement rather than per value.
	labels []string
	// index is the position of the current value in an INSERT row.
	index int
}

func newStatement(cols *Columns) *statement {
	return &statement{cols: cols, tables: map[string][]string{}}
}

// label returns the label for a value at the current position, or "" if
// the position is not a sensitive column of an INSERT row.
func (s *statement) label() string {
	if s.kind != insert || s.phase != values || s.depth != 1 || s.index >= len(s.labels) {
		return ""
	}

	return s.labels[s.index]
}

// observe moves the statement on past tok, a token that is not whitespace,
// a comment or a directive marker.
func (s *statement) observe(tok Token) {
	if isPunct(tok, ";") {
		*s = statement{cols: s.cols, tables: s.tables}
		return
	}

	switch s.kind {
	case atStart:
		switch {
		case isKeyword(tok, "CREATE"):
			s.kind = afterCreate
		case isKeyword(tok, "INSERT", "REPLACE"):
			s.kind = insert
		default:
			s.kind = otherStatement
		}
	case afterCreate:
		switch {
		case isKeyword(tok, "TEMPORARY"):
		case isKeyword(tok, "TABLE"):
			s.kind = createTable
		default:
			s.kind = otherStatement
		}
	case createTable:
		s.observeCreate(tok)
	case insert:
		s.observeInsert(tok)
	}
}

// constraintKeywords start a CREATE TABLE definition that is not a column.
var constraintKeywords = []string{"PRIMARY", "KEY", "INDEX", "UNIQUE", "CONSTRAINT", "FOREIGN", "FULLTEXT", "SPATIAL", "CHECK"}

func (s *statement) observeCreate(tok Token) {
	switch s.phase {
	case tableName:
		switch {
		case isKeyword(tok, "IF", "NOT", "EXISTS"), isPunct(tok, "."):
		case isName(tok):
			s.table = nameOf(tok)
		case isPunct(tok, "("):
			s.phase, s.depth, s.newColumn = columnList, 1, true
		default:
			s.phase = finished
		}
	case columnList:
		newColumn := s.newColumn
		s.newColumn = false
		switch {
		case isPunct(tok, "("):
			s.depth++
		case isPunct(tok, ")"):
			s.depth--
			if s.depth == 0 {
				s.tables[s.table] = s.columns
				s.phase = finished
			}
		case s.depth == 1 && isPunct(tok, ","):
			s.newColumn = true
		case newColumn && isName(tok) && !isKeyword(tok, constraintKeywords...):
			s.columns = append(s.columns, nameOf(tok))
		}
	}
}

// insertModifiers may come between INSERT and the table name.
var insertModifiers = []string{"LOW_PRIORITY", "DELAYED", "HIGH_PRIORITY", "IGNORE", "INTO"}

func (s *statement) observeInsert(tok Token) {
	switch s.phase {
	case tableName:
		switch {
		case isKeyword(tok, insertModifiers...):
		case isName(tok):
			s.table, s.phase = nameOf(tok), afterName
		default:
			s.phase = finished
		}
	case afterName:
		switch {
		case isPunct(tok, "."):
			s.phase = tableName
		case isPunct(tok, "(") && !s.explicit:
			s.phase, s.explicit = columnList, true
		case isKeyword(tok, "VALUES", "VALUE"):
			s.phase = values
			if !s.explicit {
				s.columns = s.tables[s.table]
			}
			s.labels = make([]string, len(s.columns))
			for i, column := range s.columns {
				s.labels[i], _ = s.cols.Label(s.table, column)
			}
		default:
			// SET, SELECT and PARTITION forms are left to pattern rules
			s.phase = finished
		}
	case columnList:
		switch {
		case isPunct(tok, ")"):
			s.phase = afterName
		case isName(tok):
			s.columns = append(s.columns, nameOf(tok))
		}
	case values:
		s.observeRow(tok)
	}
}

// observeRow tracks depth and value position through VALUES rows.
func (s *statement) observeRow(tok Token) {
	switch {
	case isPunct(tok, "("):
		s.depth++
		if s.depth == 1 {
			s.index = 0
		}
	case isPunct(tok, ")"):
		s.depth = max(s.depth-1, 0)
	case s.depth == 1 && isPunct(tok, ","):
		s.index++
	case s.depth == 0 && tok.Kind == Word && !isKeyword(tok, "ROW"):
		// ON DUPLICATE KEY UPDATE, or an AS alias
		s.phase = finished
	}
}

func isPunct(tok Token, p string) bool {
	return tok.Kind == Punct && tok.Text == p
}

func isKeyword(tok Token, words ...string) bool {
	if tok.Kind != Word {
		return false
	}
	for _, w := range words {
		if strings.EqualFold(tok.Text, w) {
			return true
		}
	}

	return false
}

func isName(tok Token) bool {
	return tok.Kind == Word || tok.Kind == Identifier
}

// nameOf returns a table or column name in lower case, so lookups ignore
// case as MySQL does for column names.
func nameOf(tok Token) string {
	return strings.ToLower(tok.Value())
}
