package sqlscan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// DefaultMaxToken is the longest token, such as one string value, that a
// Tokenizer accepts when Options sets no limit. A LONGTEXT or BLOB value
// larger than this is an error rather than being split, because a split
// could cut a secret in two.
const DefaultMaxToken = 16 << 20

// Options limits how much a Tokenizer holds in memory. Zero values use the
// defaults.
type Options struct {
	MaxToken int
}

// Tokenizer reads SQL dump tokens from a stream. Only the token being read
// is held in memory.
type Tokenizer struct {
	r        *bufio.Reader
	dialect  dialect
	maxToken int
	line     int
	text     strings.Builder
	// inDirective is set between /*! and its closing */.
	inDirective bool
	// introducer is a pending _charset word that the next literal takes,
	// if only whitespace and comments come between them.
	introducer string

	// PostgreSQL only: atLineStart is set after a line break, where a
	// backslash starts a meta-command; copy follows the statement for COPY
	// ... FROM stdin; copyPending is set from its ";" to the line break,
	// and inCopy from there to the "\." line.
	atLineStart bool
	copy        copyTracker
	copyPending bool
	inCopy      bool
}

// NewMySQL returns a Tokenizer for MySQL and MariaDB dumps, with the default
// SQL mode: backslash escapes on, double quotes delimiting strings.
func NewMySQL(r io.Reader, opts Options) *Tokenizer {
	maxToken := opts.MaxToken
	if maxToken <= 0 {
		maxToken = DefaultMaxToken
	}

	return &Tokenizer{r: bufio.NewReaderSize(r, 64<<10), maxToken: maxToken, line: 1, atLineStart: true}
}

// errTooLong is what read returns past MaxToken; Next adds the line.
var errTooLong = errors.New("token too long")

// Next returns the next token, or io.EOF after the last one.
func (t *Tokenizer) Next() (Token, error) {
	t.text.Reset()
	line := t.line

	c, err := t.r.ReadByte()
	if err != nil {
		return Token{}, err
	}
	t.text.WriteByte(c)

	tok, err := t.scan(c)
	if errors.Is(err, errTooLong) {
		return Token{}, fmt.Errorf("token at line %d is longer than %d bytes", line, t.maxToken)
	}
	if err != nil {
		return Token{}, err
	}

	tok.Text = t.text.String()
	tok.Line = line
	tok.dialect = t.dialect
	t.line += strings.Count(tok.Text, "\n")
	t.trackIntroducer(&tok)
	if t.dialect == postgres {
		t.trackPostgres(tok)
	}

	return tok, nil
}

// trackPostgres notes line starts and the start of COPY data after tok.
func (t *Tokenizer) trackPostgres(tok Token) {
	endsLine := strings.HasSuffix(tok.Text, "\n")
	t.atLineStart = endsLine
	switch tok.Kind {
	case Whitespace, Comment, Meta, CopyData, CopyEnd:
	default:
		t.trackCopy(tok)
	}
	if t.copyPending && endsLine {
		t.copyPending, t.inCopy = false, true
	}
}

// trackIntroducer gives a literal the _charset word before it, and
// remembers a new one.
func (t *Tokenizer) trackIntroducer(tok *Token) {
	switch tok.Kind {
	case Whitespace, Comment:
		return
	case String, Hex, Bit:
		if t.introducer != "" {
			tok.Charset = t.introducer
		}
	}
	t.introducer = ""
	if tok.Kind == Word && len(tok.Text) > 1 && tok.Text[0] == '_' {
		t.introducer = tok.Text[1:]
	}
}

func (t *Tokenizer) scan(c byte) (Token, error) {
	if t.dialect == postgres {
		if tok, ok, err := t.scanPostgres(c); ok {
			return tok, err
		}
	}

	backslashes := t.dialect == mysql
	switch {
	case isSpace(c) && t.copyPending:
		return Token{Kind: Whitespace}, t.spaceToLineEnd(c)
	case isSpace(c):
		return Token{Kind: Whitespace}, t.while(isSpace)
	case c == '#':
		return Token{Kind: Comment}, t.lineComment()
	case c == '-' && t.peekIs(0, '-') && (t.peekEOF(1) || t.peekMatches(1, isSpace)):
		return Token{Kind: Comment}, t.lineComment()
	case c == '/' && t.peekIs(0, '*'):
		return t.blockComment()
	case c == '*' && t.inDirective && t.peekIs(0, '/'):
		t.inDirective = false
		return Token{Kind: DirectiveEnd}, t.take(1)
	case c == '\'' || c == '"':
		unterminated, err := t.quoted(c, backslashes)
		return Token{Kind: String, Unterminated: unterminated}, err
	case c == '`':
		unterminated, err := t.quoted(c, false)
		return Token{Kind: Identifier, Unterminated: unterminated}, err
	case c == '0' && (t.peekIs(0, 'x') || t.peekIs(0, 'X')) && t.peekMatches(1, isHex):
		return t.prefixed(Hex, isHex)
	case c == '0' && (t.peekIs(0, 'b') || t.peekIs(0, 'B')) && t.peekMatches(1, isBit):
		return t.prefixed(Bit, isBit)
	case isDigit(c) || (c == '.' && t.peekMatches(0, isDigit)):
		return t.number(c == '.')
	case isWord(c):
		return t.word(c)
	}

	return Token{Kind: Punct}, nil
}

// blockComment reads "/*" onwards: an executable comment opener, or a
// comment to "*/" (or to the end of input).
func (t *Tokenizer) blockComment() (Token, error) {
	if err := t.take(1); err != nil {
		return Token{}, err
	}
	if t.peekIs(0, '!') && !t.inDirective {
		t.inDirective = true
		if err := t.take(1); err != nil {
			return Token{}, err
		}
		return Token{Kind: DirectiveStart}, t.while(isDigit)
	}

	for {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return Token{Kind: Comment, Unterminated: true}, nil
		}
		if err != nil {
			return Token{}, err
		}
		if c == '*' && t.peekIs(0, '/') {
			return Token{Kind: Comment}, t.take(1)
		}
	}
}

// quoted reads the rest of a quoted token whose opening quote was q. A
// doubled quote stands for one, and in strings a backslash escapes the next
// byte. It reports whether input ended first.
func (t *Tokenizer) quoted(q byte, backslashes bool) (bool, error) {
	for {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		switch {
		case c == '\\' && backslashes:
			if _, err := t.read(); errors.Is(err, io.EOF) {
				return true, nil
			} else if err != nil {
				return false, err
			}
		case c == q:
			if !t.peekIs(0, q) {
				return false, nil
			}
			if err := t.take(1); err != nil {
				return false, err
			}
		}
	}
}

// prefixed reads the rest of a 0x or 0b literal.
func (t *Tokenizer) prefixed(kind Kind, digit func(byte) bool) (Token, error) {
	if err := t.take(1); err != nil {
		return Token{}, err
	}
	if err := t.while(digit); err != nil {
		return Token{}, err
	}

	return t.wordTail(Token{Kind: kind})
}

// number reads digits, an optional fraction and an optional exponent. A
// letter straight after makes it a word, as MySQL allows names like 1abc.
// fraction is set when the number started with its decimal point.
func (t *Tokenizer) number(fraction bool) (Token, error) {
	if err := t.while(isDigit); err != nil {
		return Token{}, err
	}
	if t.peekIs(0, '.') && !fraction {
		if err := t.take(1); err != nil {
			return Token{}, err
		}
		if err := t.while(isDigit); err != nil {
			return Token{}, err
		}
	}
	if (t.peekIs(0, 'e') || t.peekIs(0, 'E')) && (t.peekMatches(1, isDigit) || ((t.peekIs(1, '+') || t.peekIs(1, '-')) && t.peekMatches(2, isDigit))) {
		if err := t.take(2); err != nil {
			return Token{}, err
		}
		if err := t.while(isDigit); err != nil {
			return Token{}, err
		}
	}

	return t.wordTail(Token{Kind: Number})
}

// wordTail turns tok into a Word if word characters follow it directly.
func (t *Tokenizer) wordTail(tok Token) (Token, error) {
	if !t.peekMatches(0, isWord) {
		return tok, nil
	}

	return Token{Kind: Word}, t.while(isWord)
}

// word reads a keyword or name, or a whole X'...', B'...' or N'...'
// literal when the name is that prefix.
func (t *Tokenizer) word(first byte) (Token, error) {
	if t.peekIs(0, '\'') {
		switch first {
		case 'x', 'X':
			return t.literal(Hex)
		case 'b', 'B':
			return t.literal(Bit)
		case 'n', 'N':
			if err := t.take(1); err != nil {
				return Token{}, err
			}
			unterminated, err := t.quoted('\'', t.dialect == mysql)
			return Token{Kind: String, Charset: "N", Unterminated: unterminated}, err
		}
	}

	if err := t.while(isWord); err != nil {
		return Token{}, err
	}
	if strings.EqualFold(t.text.String(), "NULL") {
		return Token{Kind: Null}, nil
	}

	return Token{Kind: Word}, nil
}

// literal reads the quoted part of X'..' or B'..', which has no escapes.
func (t *Tokenizer) literal(kind Kind) (Token, error) {
	if err := t.take(1); err != nil {
		return Token{}, err
	}
	for {
		c, err := t.read()
		if errors.Is(err, io.EOF) {
			return Token{Kind: kind, Unterminated: true}, nil
		}
		if err != nil || c == '\'' {
			return Token{Kind: kind}, err
		}
	}
}

// read consumes one byte into the token.
func (t *Tokenizer) read() (byte, error) {
	c, err := t.r.ReadByte()
	if err != nil {
		return 0, err
	}
	if t.text.Len() >= t.maxToken {
		return 0, errTooLong
	}
	t.text.WriteByte(c)

	return c, nil
}

// take consumes n bytes that peeking has already shown are there.
func (t *Tokenizer) take(n int) error {
	for range n {
		if _, err := t.read(); err != nil {
			return err
		}
	}

	return nil
}

// while consumes bytes as long as ok accepts them.
func (t *Tokenizer) while(ok func(byte) bool) error {
	for t.peekMatches(0, ok) {
		if _, err := t.read(); err != nil {
			return err
		}
	}

	return nil
}

// lineComment consumes the rest of a line comment, leaving the line break
// for the whitespace token after it.
func (t *Tokenizer) lineComment() error {
	return t.while(func(c byte) bool { return c != '\n' })
}

// peek returns the byte i places after the last one read, if there is one.
func (t *Tokenizer) peek(i int) (byte, bool) {
	b, _ := t.r.Peek(i + 1)
	if len(b) <= i {
		return 0, false
	}

	return b[i], true
}

func (t *Tokenizer) peekIs(i int, c byte) bool {
	b, ok := t.peek(i)
	return ok && b == c
}

func (t *Tokenizer) peekMatches(i int, ok func(byte) bool) bool {
	b, there := t.peek(i)
	return there && ok(b)
}

func (t *Tokenizer) peekEOF(i int) bool {
	_, ok := t.peek(i)
	return !ok
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isBit(c byte) bool { return c == '0' || c == '1' }

// isWord reports whether c can be part of an unquoted name. Bytes of
// multi-byte UTF-8 characters count, as MySQL allows them in names.
func isWord(c byte) bool {
	return c == '_' || c == '$' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}
