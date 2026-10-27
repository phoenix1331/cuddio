package sqlscan

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// NewPostgres returns a Tokenizer for pg_dump output, with
// standard_conforming_strings on (pg_dump's setting): backslashes are
// literal except in E'...' strings, double quotes delimit names. psql
// meta-commands and the data lines of COPY ... FROM stdin blocks come back
// as Meta, CopyData and CopyEnd tokens.
func NewPostgres(r io.Reader, opts Options) *Tokenizer {
	t := NewMySQL(r, opts)
	t.dialect = Postgres

	return t
}

// scanPostgres reads the tokens PostgreSQL reads differently from MySQL. It
// reports false when c starts a token the shared rules handle.
func (t *Tokenizer) scanPostgres(c byte) (Token, bool, error) {
	switch {
	case t.inCopy:
		tok, err := t.copyLine(c)
		return tok, true, err
	case c == '\\' && t.atLineStart:
		return Token{Kind: Meta}, true, t.lineComment()
	case c == '-' && t.peekIs(0, '-'):
		return Token{Kind: Comment}, true, t.lineComment()
	case c == '/' && t.peekIs(0, '*'):
		tok, err := t.nestedComment()
		return tok, true, err
	case c == '"':
		unterminated, err := t.quoted(c, false)
		return Token{Kind: Identifier, Unterminated: unterminated}, true, err
	case c == '$':
		if n, ok := t.dollarTag(); ok {
			tok, err := t.dollarQuoted(n)
			return tok, true, err
		}
	case (c == 'e' || c == 'E') && t.peekIs(0, '\''):
		if err := t.take(1); err != nil {
			return Token{}, true, err
		}
		unterminated, err := t.quoted('\'', true)
		return Token{Kind: String, Charset: "E", Unterminated: unterminated}, true, err
	case c == '#' || c == '`':
		return Token{Kind: Punct}, true, nil
	}

	return Token{}, false, nil
}

// nestedComment reads a block comment, which PostgreSQL lets nest.
func (t *Tokenizer) nestedComment() (Token, error) {
	if err := t.take(1); err != nil {
		return Token{}, err
	}
	for depth := 1; ; {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return Token{Kind: Comment, Unterminated: true}, nil
		}
		if err != nil {
			return Token{}, err
		}
		switch {
		case c == '*' && t.peekIs(0, '/'):
			depth--
		case c == '/' && t.peekIs(0, '*'):
			depth++
		default:
			continue
		}
		if err := t.take(1); err != nil {
			return Token{}, err
		}
		if depth == 0 {
			return Token{Kind: Comment}, nil
		}
	}
}

// dollarTag reports whether the "$" just read opens a dollar quote, and the
// length of the tag between the two "$" signs. A tag is like a name, but
// cannot start with a digit, so $1 stays a parameter.
func (t *Tokenizer) dollarTag() (int, bool) {
	for i := 0; ; i++ {
		b, ok := t.peek(i)
		switch {
		case ok && b == '$':
			return i, true
		case !ok || !isWord(b) || (i == 0 && isDigit(b)):
			return 0, false
		}
	}
}

// dollarQuoted reads $tag$ ... $tag$, whose body is taken as it stands.
func (t *Tokenizer) dollarQuoted(tag int) (Token, error) {
	if err := t.take(tag + 1); err != nil {
		return Token{}, err
	}
	closer := t.text.String()
	for {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return Token{Kind: String, Unterminated: true}, nil
		}
		if err != nil {
			return Token{}, err
		}
		if text := t.text.String(); c == '$' && len(text) >= 2*len(closer) && strings.HasSuffix(text, closer) {
			return Token{Kind: String}, nil
		}
	}
}

// copyLine reads the rest of a line of COPY data.
func (t *Tokenizer) copyLine(c byte) (Token, error) {
	for c != '\n' {
		var err error
		c, err = t.read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Token{}, err
		}
	}

	if line, _ := splitLineEnding(t.text.String()); line == `\.` {
		t.inCopy = false
		return Token{Kind: CopyEnd}, nil
	}

	return Token{Kind: CopyData}, nil
}

// spaceToLineEnd reads whitespace up to and including the next line break,
// where the data of a COPY ... FROM stdin starts.
func (t *Tokenizer) spaceToLineEnd(c byte) error {
	if c == '\n' {
		return nil
	}
	if err := t.while(func(b byte) bool { return isSpace(b) && b != '\n' }); err != nil {
		return err
	}
	if t.peekIs(0, '\n') {
		return t.take(1)
	}

	return nil
}

// trackCopy follows statements closely enough to see COPY ... FROM stdin,
// whose data starts on the line after its ";". tok is significant: not
// whitespace, a comment or a meta-command.
func (t *Tokenizer) trackCopy(tok Token) {
	if tok.Kind == Punct && tok.Text == ";" {
		t.copyPending = t.copy.stdin
		t.copy = copyTracker{}
		return
	}
	if !t.copy.started {
		t.copy.started, t.copy.isCopy = true, isKeyword(tok, "COPY")
	}
	if !t.copy.isCopy {
		return
	}
	if t.copy.afterFrom && isKeyword(tok, "STDIN") {
		t.copy.stdin = true
	}
	t.copy.afterFrom = isKeyword(tok, "FROM")
}

// copyTracker is what trackCopy knows about the current statement.
type copyTracker struct {
	started, isCopy, afterFrom, stdin bool
}

// splitLineEnding splits a final "\n" or "\r\n" off line.
func splitLineEnding(line string) (body, ending string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}

	return line, ""
}

// dollarBody returns the text between a dollar-quoted string's tags.
func dollarBody(text string, unterminated bool) string {
	tag := text[:strings.IndexByte(text[1:], '$')+2]
	body := text[len(tag):]
	if !unterminated {
		body = body[:len(body)-len(tag)]
	}

	return body
}

// unquoteE decodes a PostgreSQL E'...' string: doubled quotes, and backslash
// escapes including octal (\101), hex (\x41) and Unicode (\u0041,
// \U00000041). An unknown escape stands for the byte after the backslash.
func unquoteE(text string, unterminated bool) string {
	body := text[strings.IndexByte(text, '\'')+1:]
	if !unterminated {
		body = body[:len(body)-1]
	}

	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\'' && i+1 < len(body) && body[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		if c != '\\' || i+1 == len(body) {
			b.WriteByte(c)
			continue
		}
		n := decodeE(&b, body[i+1:])
		i += n
	}

	return b.String()
}

// decodeE writes the character an E'...' escape stands for, given the text
// after its backslash, and returns how many bytes of rest it used.
func decodeE(b *strings.Builder, rest string) int {
	switch c := rest[0]; {
	case c >= '0' && c <= '7':
		n := prefixLen(rest, 3, isOctal)
		v, _ := strconv.ParseUint(rest[:n], 8, 8)
		b.WriteByte(byte(v))
		return n
	case c == 'x' && prefixLen(rest[1:], 2, isHex) > 0:
		n := prefixLen(rest[1:], 2, isHex)
		v, _ := strconv.ParseUint(rest[1:1+n], 16, 8)
		b.WriteByte(byte(v))
		return 1 + n
	case (c == 'u' && prefixLen(rest[1:], 4, isHex) == 4) || (c == 'U' && prefixLen(rest[1:], 8, isHex) == 8):
		n := 4
		if c == 'U' {
			n = 8
		}
		r := utf8.RuneError
		if v, _ := strconv.ParseInt(rest[1:1+n], 16, 32); utf8.ValidRune(rune(v)) {
			r = rune(v)
		}
		b.WriteRune(r)
		return 1 + n
	}
	b.WriteString(pgEscapes(rest[0]))

	return 1
}

// pgEscapes decodes a one-letter E'...' escape.
func pgEscapes(c byte) string {
	switch c {
	case 'b':
		return "\b"
	case 'f':
		return "\f"
	case 'n':
		return "\n"
	case 'r':
		return "\r"
	case 't':
		return "\t"
	}

	return string(c)
}

// prefixLen counts how many of the first max bytes of s satisfy ok, up to
// the first that does not.
func prefixLen(s string, maxLen int, ok func(byte) bool) int {
	n := 0
	for n < maxLen && n < len(s) && ok(s[n]) {
		n++
	}

	return n
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }
