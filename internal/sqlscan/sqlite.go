package sqlscan

import (
	"errors"
	"io"
)

// NewSQLite returns a Tokenizer for sqlite3 .dump output: backslashes are
// literal, double quotes and square brackets delimit names, and comments do
// not nest.
func NewSQLite(r io.Reader, opts Options) *Tokenizer {
	t := NewMySQL(r, opts)
	t.dialect = SQLite

	return t
}

// scanSQLite reads the tokens SQLite reads differently from MySQL. It
// reports false when c starts a token the shared rules handle.
func (t *Tokenizer) scanSQLite(c byte) (Token, bool, error) {
	switch {
	case c == '-' && t.peekIs(0, '-'):
		return Token{Kind: Comment}, true, t.lineComment()
	case c == '"':
		unterminated, err := t.quoted(c, false)
		return Token{Kind: Identifier, Unterminated: unterminated}, true, err
	case c == '[':
		unterminated, err := t.bracketed()
		return Token{Kind: Identifier, Unterminated: unterminated}, true, err
	case c == '#':
		return Token{Kind: Punct}, true, nil
	}

	return Token{}, false, nil
}

// bracketed reads the rest of a [name], which has no escapes.
func (t *Tokenizer) bracketed() (bool, error) {
	for {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		if err != nil || c == ']' {
			return false, err
		}
	}
}
