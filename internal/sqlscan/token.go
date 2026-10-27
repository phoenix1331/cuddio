// Package sqlscan reads SQL dumps as a stream of tokens, so a dump of any
// size is cleaned in bounded memory and everything cuddio does not change is
// written back byte for byte.
package sqlscan

import (
	"fmt"
	"strings"
)

// Kind is what a token is.
type Kind int

// Token kinds. Concatenating every token's Text gives back the input.
const (
	// Whitespace is a run of spaces, tabs and line breaks.
	Whitespace Kind = iota
	// Comment is "-- ..." or "# ..." to the end of the line (the line
	// break is not included), or "/* ... */".
	Comment
	// DirectiveStart opens a MySQL executable comment, such as "/*!40101".
	// The tokens up to the matching DirectiveEnd are SQL that MySQL runs.
	DirectiveStart
	// DirectiveEnd is the "*/" closing an executable comment.
	DirectiveEnd
	// String is a quoted string literal, including its quotes and any
	// N or E prefix: 'it''s', "a\"b" (MySQL), N'x', E'a\n' and $$x$$
	// (PostgreSQL).
	String
	// Identifier is a quoted name: `users` (MySQL), "users" (PostgreSQL).
	Identifier
	// Hex is a hexadecimal literal: 0x89AB or X'89AB'.
	Hex
	// Bit is a bit literal: 0b0101 or b'0101'.
	Bit
	// Null is the NULL keyword, in any case.
	Null
	// Number is a numeric literal: 42, 3.14, 1e-5.
	Number
	// Word is a keyword or unquoted name: INSERT, users, _binary.
	Word
	// Punct is one byte of anything else: ( ) , ; = and so on.
	Punct
	// Meta is a psql meta-command line such as "\connect shop", without
	// its line break.
	Meta
	// CopyData is one line of a PostgreSQL COPY ... FROM stdin block,
	// line break included.
	CopyData
	// CopyEnd is the "\." line that ends a COPY block, line break included.
	CopyEnd
)

var kindNames = [...]string{"Whitespace", "Comment", "DirectiveStart", "DirectiveEnd", "String", "Identifier", "Hex", "Bit", "Null", "Number", "Word", "Punct", "Meta", "CopyData", "CopyEnd"}

// dialect is the SQL flavour a Tokenizer reads.
type dialect int

const (
	mysql dialect = iota
	postgres
)

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}

	return fmt.Sprintf("Kind(%d)", int(k))
}

// Token is one piece of SQL.
type Token struct {
	Kind Kind
	// Text is the token exactly as it appeared in the input.
	Text string
	// Line is the 1-based line the token starts on.
	Line int
	// Charset is the introducer before a String, Hex or Bit literal
	// without its underscore ("binary" for _binary 'x'), "N" for N'x', or
	// "E" for a PostgreSQL E'x'.
	Charset string
	// Unterminated is set on a String, Identifier or Comment that the input
	// ended inside.
	Unterminated bool
	dialect      dialect
}

// Value returns a String token's contents with quotes removed and escapes
// decoded as its dialect reads them, or an Identifier's name. MySQL strings
// take backslash escapes and doubled quotes; PostgreSQL strings take doubled
// quotes only, E'...' strings backslash escapes as well, and dollar-quoted
// strings nothing. Other tokens return Text.
func (t Token) Value() string {
	switch {
	case t.Kind == Identifier:
		return unquote(t.Text, t.Unterminated, false)
	case t.Kind != String:
		return t.Text
	case t.dialect == mysql:
		return unquote(t.Text, t.Unterminated, true)
	case t.Text[0] == '$':
		return dollarBody(t.Text, t.Unterminated)
	case t.Charset == "E":
		return unquoteE(t.Text, t.Unterminated)
	}

	return unquote(t.Text, t.Unterminated, false)
}

// unquote strips the prefix, opening quote and closing quote from a quoted
// token and decodes its contents.
func unquote(text string, unterminated, backslashes bool) string {
	start := strings.IndexAny(text, "'\"`")
	quote := text[start]
	body := text[start+1:]
	if !unterminated {
		body = body[:len(body)-1]
	}

	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == quote && i+1 < len(body) && body[i+1] == quote:
			b.WriteByte(quote)
			i++
		case c == '\\' && backslashes && i+1 < len(body):
			i++
			b.WriteString(unescape(body[i]))
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

// unescape decodes the byte after a backslash. \% and \_ keep their
// backslash, as MySQL does, because they only mean something in LIKE.
func unescape(c byte) string {
	switch c {
	case '0':
		return "\x00"
	case 'b':
		return "\b"
	case 'n':
		return "\n"
	case 'r':
		return "\r"
	case 't':
		return "\t"
	case 'Z':
		return "\x1a"
	case '%', '_':
		return "\\" + string(c)
	}

	return string(c)
}
