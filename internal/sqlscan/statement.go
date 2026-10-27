package sqlscan

import (
	"strconv"
	"strings"
)

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
	copyStatement
)

// phase is where a CREATE TABLE, INSERT or COPY has got to.
type phase int

const (
	// tableName: reading the table name.
	tableName phase = iota
	// afterName: CREATE TABLE expects "(", INSERT a column list or VALUES,
	// COPY a column list or FROM.
	afterName
	// columnList: inside CREATE TABLE's definitions or a column list.
	columnList
	// overriding: INSERT ... OVERRIDING SYSTEM VALUE, up to VALUE.
	overriding
	// values: INSERT rows.
	values
	// copyFrom: COPY ... FROM, STDIN may follow.
	copyFrom
	// copyStdin: COPY ... FROM STDIN, options may follow.
	copyStdin
	// finished: nothing more to track.
	finished
)

// copyRows is what the rows of a COPY ... FROM stdin block need: each
// field's label and length, or raw when the COPY has options, so its rows
// may not be tab-separated text.
type copyRows struct {
	fields []field
	raw    bool
}

// field is what cleaning needs to know about a column: its label, "" when
// it is not sensitive, and the most characters it holds, 0 when unknown.
type field struct {
	label  string
	maxLen int
}

// table is what CREATE TABLE said about a table: its column names in order,
// and the length of each char or varchar column.
type table struct {
	columns []string
	lengths map[string]int
}

// lengthTypes are column types whose "(n)" is a length in characters.
var lengthTypes = []string{"CHAR", "VARCHAR", "CHARACTER", "VARYING", "NCHAR", "NVARCHAR"}

// statement follows the statement being cleaned closely enough to know
// which column each INSERT value or COPY field belongs to. It keeps the
// column names of every table created so far, and nothing else from
// earlier statements.
type statement struct {
	cols   *Columns
	tables map[string]table
	// copy describes the COPY data block being read, from the ";" of its
	// COPY statement to its "\." line.
	copy *copyRows

	kind    statementKind
	phase   phase
	depth   int
	table   string
	columns []string
	// explicit is set once an INSERT or COPY has its own column list.
	explicit bool
	// newColumn is set where a CREATE TABLE definition starts, so a name
	// there is a column unless it is a keyword such as PRIMARY.
	newColumn bool
	// lengths collects CREATE TABLE's char and varchar lengths; typeWord is
	// the last word of the current definition's type, and lengthNext is
	// set inside its "(" when that word is one of lengthTypes.
	lengths    map[string]int
	typeWord   Token
	lengthNext bool
	// options is set when a COPY ... FROM STDIN has anything after STDIN.
	options bool
	// fields describes each INSERT column, worked out once per statement
	// rather than per value.
	fields []field
	// index is the position of the current value in an INSERT row.
	index int
	// afterWrapper is set just after a replace or unistr call name in a
	// row; wrapped is set inside that call's parentheses, and arg counts
	// its arguments.
	afterWrapper, wrapped bool
	arg                   int
}

func newStatement(cols *Columns) *statement {
	return &statement{cols: cols, tables: map[string]table{}}
}

// field returns the column at the current position, or a zero field if the
// position is not a value of an INSERT row. A value is a literal in the row
// itself, or the first argument of replace() or unistr(), which sqlite3
// .dump wraps around strings holding line breaks.
func (s *statement) field() field {
	if s.kind != insert || s.phase != values || s.index >= len(s.fields) {
		return field{}
	}
	if s.depth == 1 || (s.depth == 2 && s.wrapped && s.arg == 0) {
		return s.fields[s.index]
	}

	return field{}
}

// columnFields describes each column the statement fills: its own column
// list, or the table's columns from CREATE TABLE. Lengths come from CREATE
// TABLE either way.
func (s *statement) columnFields() []field {
	t := s.tables[s.table]
	if !s.explicit {
		s.columns = t.columns
	}
	fields := make([]field, len(s.columns))
	for i, column := range s.columns {
		fields[i].label, _ = s.cols.Label(s.table, column)
		fields[i].maxLen = t.lengths[column]
	}

	return fields
}

// observe moves the statement on past tok, a token that is not whitespace,
// a comment, a meta-command, COPY data or a directive marker.
func (s *statement) observe(tok Token) {
	if isPunct(tok, ";") {
		var rows *copyRows
		if s.kind == copyStatement && s.phase == copyStdin {
			rows = &copyRows{fields: s.columnFields(), raw: s.options}
		}
		*s = statement{cols: s.cols, tables: s.tables, copy: rows}
		return
	}

	switch s.kind {
	case atStart:
		switch {
		case isKeyword(tok, "CREATE"):
			s.kind = afterCreate
		case isKeyword(tok, "INSERT", "REPLACE"):
			s.kind = insert
		case isKeyword(tok, "COPY"):
			s.kind = copyStatement
		default:
			s.kind = otherStatement
		}
	case afterCreate:
		switch {
		case isKeyword(tok, "OR", "REPLACE", "TEMPORARY", "TEMP", "UNLOGGED", "GLOBAL", "LOCAL"):
		case isKeyword(tok, "TABLE"):
			s.kind = createTable
		default:
			s.kind = otherStatement
		}
	case createTable:
		s.observeCreate(tok)
	case insert:
		s.observeInsert(tok)
	case copyStatement:
		s.observeCopy(tok)
	}
}

// observeName reads a table name, which may be qualified (shop.users), and
// reports whether tok was part of one. A name ends the tableName phase; a
// "." after it starts the phase again.
func (s *statement) observeName(tok Token) bool {
	switch {
	case s.phase == tableName && isName(tok):
		s.table, s.phase = nameOf(tok), afterName
	case s.phase == afterName && isPunct(tok, "."):
		s.phase = tableName
	default:
		return false
	}

	return true
}

// constraintKeywords start a CREATE TABLE definition that is not a column.
var constraintKeywords = []string{"PRIMARY", "KEY", "INDEX", "UNIQUE", "CONSTRAINT", "FOREIGN", "FULLTEXT", "SPATIAL", "CHECK", "EXCLUDE", "LIKE"}

func (s *statement) observeCreate(tok Token) {
	switch {
	case s.phase == tableName && isKeyword(tok, "IF", "NOT", "EXISTS"):
	case s.observeName(tok):
	case s.phase == afterName && isPunct(tok, "("):
		s.phase, s.depth, s.newColumn = columnList, 1, true
		s.lengths = map[string]int{}
	case s.phase == columnList:
		s.observeDefinition(tok)
	default:
		// LIKE, AS SELECT, PARTITION OF: no column list to read
		s.phase = finished
	}
}

// observeDefinition reads CREATE TABLE's column definitions, and the
// length of each char and varchar column.
func (s *statement) observeDefinition(tok Token) {
	newColumn := s.newColumn
	s.newColumn = false
	lengthNext := s.lengthNext
	s.lengthNext = false
	switch {
	case isPunct(tok, "("):
		s.depth++
		s.lengthNext = s.depth == 2 && isKeyword(s.typeWord, lengthTypes...)
	case isPunct(tok, ")"):
		s.depth--
		if s.depth == 0 {
			s.tables[s.table] = table{columns: s.columns, lengths: s.lengths}
			s.phase = finished
		}
	case s.depth == 1 && isPunct(tok, ","):
		s.newColumn = true
	case newColumn && isName(tok) && !isKeyword(tok, constraintKeywords...):
		s.columns = append(s.columns, nameOf(tok))
		s.typeWord = Token{}
	case lengthNext && tok.Kind == Number && len(s.columns) > 0:
		if n, err := strconv.Atoi(tok.Text); err == nil {
			s.lengths[s.columns[len(s.columns)-1]] = n
		}
	case s.depth == 1 && tok.Kind == Word:
		s.typeWord = tok
	}
}

// insertModifiers may come between INSERT and the table name.
var insertModifiers = []string{"LOW_PRIORITY", "DELAYED", "HIGH_PRIORITY", "IGNORE", "INTO"}

func (s *statement) observeInsert(tok Token) {
	switch {
	case s.phase == tableName && isKeyword(tok, insertModifiers...):
	case s.observeName(tok):
	case s.phase == afterName && isPunct(tok, "(") && !s.explicit:
		s.phase, s.explicit = columnList, true
	case s.phase == afterName && isKeyword(tok, "OVERRIDING"):
		s.phase = overriding
	case s.phase == afterName && isKeyword(tok, "VALUES", "VALUE"):
		s.phase = values
		s.fields = s.columnFields()
	case s.phase == columnList:
		s.observeColumnList(tok)
	case s.phase == overriding:
		if isKeyword(tok, "VALUE") {
			s.phase = afterName
		}
	case s.phase == values:
		s.observeRow(tok)
	default:
		// SET, SELECT, DEFAULT VALUES and PARTITION forms are left to
		// pattern rules
		s.phase = finished
	}
}

func (s *statement) observeCopy(tok Token) {
	switch {
	case s.observeName(tok):
	case s.phase == afterName && isPunct(tok, "(") && !s.explicit:
		s.phase, s.explicit = columnList, true
	case s.phase == afterName && isKeyword(tok, "FROM"):
		s.phase = copyFrom
	case s.phase == columnList:
		s.observeColumnList(tok)
	case s.phase == copyFrom && isKeyword(tok, "STDIN"):
		s.phase = copyStdin
	case s.phase == copyStdin:
		s.options = true
	default:
		// COPY (query) TO, COPY ... TO STDOUT, or FROM a file
		s.phase = finished
	}
}

// observeColumnList reads an INSERT or COPY column list up to its ")".
func (s *statement) observeColumnList(tok Token) {
	switch {
	case isPunct(tok, ")"):
		s.phase = afterName
	case isName(tok):
		s.columns = append(s.columns, nameOf(tok))
	}
}

// observeRow tracks depth and value position through VALUES rows.
func (s *statement) observeRow(tok Token) {
	afterWrapper := s.depth == 1 && isKeyword(tok, "REPLACE", "UNISTR")
	defer func() { s.afterWrapper = afterWrapper }()

	switch {
	case isPunct(tok, "("):
		s.depth++
		if s.depth == 1 {
			s.index = 0
		}
		if s.depth == 2 {
			s.wrapped, s.arg = s.afterWrapper, 0
		}
	case isPunct(tok, ")"):
		s.depth = max(s.depth-1, 0)
	case s.depth == 1 && isPunct(tok, ","):
		s.index++
	case s.depth == 2 && isPunct(tok, ","):
		s.arg++
	case s.depth == 0 && tok.Kind == Word && !isKeyword(tok, "ROW"):
		// ON DUPLICATE KEY UPDATE, ON CONFLICT, or an AS alias
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
// case as MySQL does for column names and PostgreSQL for unquoted names.
func nameOf(tok Token) string {
	return strings.ToLower(tok.Value())
}
