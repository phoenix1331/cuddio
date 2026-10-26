package sqltest

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver" // value expressions in parsed statements
	pgquery "github.com/wasilibs/go-pgquery"
	_ "modernc.org/sqlite" // database/sql driver "sqlite"
)

// Dialect is an SQL dump format.
type Dialect string

// Dialects cuddio cleans.
const (
	MySQL    Dialect = "mysql"
	Postgres Dialect = "postgres"
	SQLite   Dialect = "sqlite"
)

// Check reports whether dump is valid for its dialect: it parses, and every
// row has as many values as its table has columns. SQLite dumps are imported
// into an in-memory database, so SQLite itself does the checking. Errors
// name tables and row numbers, never values.
func Check(d Dialect, dump string) error {
	switch d {
	case MySQL:
		return checkMySQL(dump)
	case Postgres:
		return checkPostgres(dump)
	case SQLite:
		db, err := OpenSQLite(dump)
		if err != nil {
			return err
		}
		return db.Close()
	}

	return fmt.Errorf("unknown dialect %q", d)
}

// delimiterBlock matches a mysql client DELIMITER ;; block, which wraps
// triggers and routines in mysqldump output. The DELIMITER lines are not SQL,
// and the TiDB parser does not support triggers or routines, so blocks are
// left out of the check; they hold code, not row data.
var delimiterBlock = regexp.MustCompile(`(?ms)^DELIMITER ;;\r?\n.*?^DELIMITER ;\r?\n`)

func checkMySQL(dump string) error {
	// Keep the newlines, so parser errors keep their line numbers
	dump = delimiterBlock.ReplaceAllStringFunc(dump, func(block string) string {
		return strings.Repeat("\n", strings.Count(block, "\n"))
	})

	stmts, _, err := parser.New().Parse(dump, "", "")
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}

	columns := map[string]int{}
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.CreateTableStmt:
			columns[s.Table.Name.L] = len(s.Cols)
		case *ast.InsertStmt:
			table := insertTable(s)
			want := len(s.Columns)
			if want == 0 {
				want = columns[table]
			}
			for i, row := range s.Lists {
				if want > 0 && len(row) != want {
					return fmt.Errorf("mysql: table %s row %d: %d values, want %d", table, i+1, len(row), want)
				}
			}
		}
	}

	return nil
}

func insertTable(s *ast.InsertStmt) string {
	source, ok := s.Table.TableRefs.Left.(*ast.TableSource)
	if !ok {
		return ""
	}
	name, ok := source.Source.(*ast.TableName)
	if !ok {
		return ""
	}

	return name.Name.L
}

// copyBlock is one COPY ... FROM stdin data block.
type copyBlock struct {
	line int
	rows []string
}

// copyHeader matches the statement that starts a COPY data block.
var copyHeader = regexp.MustCompile(`^COPY .* FROM stdin;$`)

// splitPostgres separates a pg_dump file into SQL, with psql meta-commands
// (\restrict, \connect) and COPY data removed, and the COPY data blocks.
// Removed lines become empty, so parser errors keep their line numbers.
func splitPostgres(dump string) (string, []copyBlock, error) {
	lines := strings.Split(strings.ReplaceAll(dump, "\r\n", "\n"), "\n")
	var blocks []copyBlock
	var current *copyBlock

	for i, line := range lines {
		if current != nil {
			lines[i] = ""
			if line == `\.` {
				blocks = append(blocks, *current)
				current = nil
				continue
			}
			current.rows = append(current.rows, line)
			continue
		}
		if strings.HasPrefix(line, `\`) {
			lines[i] = ""
			continue
		}
		if copyHeader.MatchString(line) {
			current = &copyBlock{line: i + 1}
		}
	}
	if current != nil {
		return "", nil, fmt.Errorf("postgres: COPY at line %d has no \\. terminator", current.line)
	}

	return strings.Join(lines, "\n"), blocks, nil
}

func checkPostgres(dump string) error {
	statements, blocks, err := splitPostgres(dump)
	if err != nil {
		return err
	}
	tree, err := pgquery.Parse(statements)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	columns := map[string]int{}
	copies := 0
	for _, raw := range tree.GetStmts() {
		stmt := raw.GetStmt()
		if create := stmt.GetCreateStmt(); create != nil {
			n := 0
			for _, elt := range create.GetTableElts() {
				if elt.GetColumnDef() != nil {
					n++
				}
			}
			columns[create.GetRelation().GetRelname()] = n
		}
		if insert := stmt.GetInsertStmt(); insert != nil {
			if err := checkInsertValues(insert, columns); err != nil {
				return err
			}
		}
		if cp := stmt.GetCopyStmt(); cp != nil && cp.GetIsFrom() && cp.GetFilename() == "" {
			if copies >= len(blocks) {
				return errors.New("postgres: COPY statement without a data block")
			}
			table := cp.GetRelation().GetRelname()
			want := len(cp.GetAttlist())
			if want == 0 {
				want = columns[table]
			}
			if err := checkCopyRows(table, blocks[copies], want); err != nil {
				return err
			}
			copies++
		}
	}

	return nil
}

func checkInsertValues(insert *pg.InsertStmt, columns map[string]int) error {
	table := insert.GetRelation().GetRelname()
	want := len(insert.GetCols())
	if want == 0 {
		want = columns[table]
	}
	for i, row := range insert.GetSelectStmt().GetSelectStmt().GetValuesLists() {
		if got := len(row.GetList().GetItems()); want > 0 && got != want {
			return fmt.Errorf("postgres: table %s row %d: %d values, want %d", table, i+1, got, want)
		}
	}

	return nil
}

// checkCopyRows checks each row of a COPY text-format block: fields split on
// unescaped tabs, as many as the table has columns, and no row ending in a
// lone backslash.
func checkCopyRows(table string, block copyBlock, want int) error {
	for i, row := range block.rows {
		fields, err := copyFields(row)
		if err != nil {
			return fmt.Errorf("postgres: table %s COPY row %d: %w", table, i+1, err)
		}
		if want > 0 && fields != want {
			return fmt.Errorf("postgres: table %s COPY row %d: %d fields, want %d", table, i+1, fields, want)
		}
	}

	return nil
}

// copyFields counts the fields in a COPY text row. A backslash escapes the
// next byte (including a tab), so it cannot end a row.
func copyFields(row string) (int, error) {
	fields := 1
	for i := 0; i < len(row); i++ {
		switch row[i] {
		case '\\':
			if i == len(row)-1 {
				return 0, errors.New("ends in a lone backslash")
			}
			i++
		case '\t':
			fields++
		}
	}

	return fields, nil
}

// OpenSQLite imports dump into a new in-memory SQLite database. The caller
// closes it.
func OpenSQLite(dump string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	// Each connection to ":memory:" is its own database
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(dump); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	return db, nil
}
