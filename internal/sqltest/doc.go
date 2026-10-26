// Package sqltest is test support for SQL scanning: Laravel-shaped fixture
// dumps, a generator for large dumps, and validity checks for MySQL,
// PostgreSQL and SQLite output. Only tests import it, so its parser and
// database libraries never reach the cuddio binary (network_test.go checks).
package sqltest
