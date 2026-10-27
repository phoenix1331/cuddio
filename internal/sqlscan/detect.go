package sqlscan

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// DetectSize is how much of the start of a dump Detect looks at.
const DetectSize = 64 << 10

// markers are text that one dialect's dump tool writes near the top of a
// dump and the others never do.
var markers = []struct {
	text    string
	dialect Dialect
}{
	{"-- MySQL dump", MySQL},
	{"-- MariaDB dump", MySQL},
	{"/*!40", MySQL},
	{"/*M!", MySQL},
	{"`", MySQL},
	{"-- PostgreSQL database dump", Postgres},
	{"\\restrict ", Postgres},
	{"SET standard_conforming_strings", Postgres},
	{"pg_catalog.", Postgres},
	{" FROM stdin;", Postgres},
	{"PRAGMA foreign_keys=OFF;", SQLite},
	{"BEGIN TRANSACTION;", SQLite},
	{"sqlite_sequence", SQLite},
}

// Detect names the dialect of a dump from its first DetectSize bytes: the
// dialect whose marker comes first, so a value further down that happens
// to hold another dialect's marker cannot change the answer. It reports
// false when no marker appears.
func Detect(header []byte) (Dialect, bool) {
	var found Dialect
	first := -1
	for _, m := range markers {
		if i := bytes.Index(header, []byte(m.text)); i >= 0 && (first < 0 || i < first) {
			found, first = m.dialect, i
		}
	}

	return found, first >= 0
}

// ParseDialect reads a dialect name, accepting mariadb, postgresql, pg and
// sqlite3 as well, in any case.
func ParseDialect(name string) (Dialect, bool) {
	switch strings.ToLower(name) {
	case "mysql", "mariadb":
		return MySQL, true
	case "postgres", "postgresql", "pg":
		return Postgres, true
	case "sqlite", "sqlite3":
		return SQLite, true
	}

	return "", false
}

// Clean copies a dump in dialect d from r to w, as CleanMySQL,
// CleanPostgres and CleanSQLite describe.
func Clean(w io.Writer, r io.Reader, d Dialect, rd Redactor, cols *Columns, opts Options) error {
	switch d {
	case MySQL:
		return CleanMySQL(w, r, rd, cols, opts)
	case Postgres:
		return CleanPostgres(w, r, rd, cols, opts)
	case SQLite:
		return CleanSQLite(w, r, rd, cols, opts)
	}

	return fmt.Errorf("unknown SQL dialect %q", string(d))
}
