// Package textscan cleans plain text and logs as a stream, one line at a
// time, so input of any size runs in bounded memory.
package textscan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// DefaultMaxLine is the longest line, including its line ending, that
	// Clean accepts when Options sets no limit.
	DefaultMaxLine = 1 << 20
	// DefaultMaxBlock is how much of a PEM block Clean buffers when Options
	// sets no limit. A 4096-bit RSA private key is about 3 KB.
	DefaultMaxBlock = 64 << 10
)

// Options limits how much Clean holds in memory. Zero values use the defaults.
type Options struct {
	MaxLine  int
	MaxBlock int
}

// Clean copies r to w, passing each line to redact without its line ending.
// A PEM block ("-----BEGIN ...-----" to "-----END ...-----") is passed as one
// chunk, line endings included, so a rule can match a whole private key.
// Line endings are written back exactly as read: "\n", "\r\n" or none on the
// last line.
//
// A line longer than MaxLine is an error rather than being split, because a
// split could cut a secret in two and let both halves through. A block that
// grows past MaxBlock, or is still open at the end of input, is passed as it
// stands.
func Clean(w io.Writer, r io.Reader, redact func(string) string, opts Options) error {
	// bufio never buffers less than 16 bytes, so neither does the line limit
	maxLine := max(orDefault(opts.MaxLine, DefaultMaxLine), 16)
	s := &scanner{
		out:      bufio.NewWriter(w),
		redact:   redact,
		maxBlock: orDefault(opts.MaxBlock, DefaultMaxBlock),
	}
	in := bufio.NewReaderSize(r, maxLine)

	for n := 1; ; n++ {
		line, err := in.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// A last line with no ending fills the buffer exactly
			if _, perr := in.Peek(1); !errors.Is(perr, io.EOF) {
				return fmt.Errorf("line %d is longer than %d bytes", n, maxLine)
			}
			err = io.EOF
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read: %w", err)
		}
		if len(line) > 0 {
			if werr := s.line(string(line)); werr != nil {
				return werr
			}
		}
		if err != nil {
			break
		}
	}

	if err := s.flushBlock(); err != nil {
		return err
	}
	if err := s.out.Flush(); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

type scanner struct {
	out      *bufio.Writer
	redact   func(string) string
	maxBlock int
	block    strings.Builder
	inBlock  bool
}

func (s *scanner) line(line string) error {
	if s.inBlock {
		s.block.WriteString(line)
		if strings.Contains(line, pemEnd) || s.block.Len() >= s.maxBlock {
			return s.flushBlock()
		}
		return nil
	}

	if opensBlock(line) {
		s.inBlock = true
		s.block.WriteString(line)
		return nil
	}

	return s.emit(line)
}

func (s *scanner) flushBlock() error {
	if !s.inBlock {
		return nil
	}

	chunk := s.block.String()
	s.block.Reset()
	s.inBlock = false

	return s.emit(chunk)
}

// emit redacts chunk without its final line ending, so a rule matching to
// the end of a value never takes the "\r" of a CRLF file with it.
func (s *scanner) emit(chunk string) error {
	body, ending := splitEnding(chunk)
	if _, err := s.out.WriteString(s.redact(body)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if _, err := s.out.WriteString(ending); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

const (
	pemBegin = "-----BEGIN "
	pemEnd   = "-----END "
)

// opensBlock reports whether line starts a PEM block it does not also close.
func opensBlock(line string) bool {
	begin := strings.LastIndex(line, pemBegin)
	return begin >= 0 && strings.LastIndex(line, pemEnd) < begin
}

func splitEnding(chunk string) (body, ending string) {
	if strings.HasSuffix(chunk, "\r\n") {
		return chunk[:len(chunk)-2], "\r\n"
	}
	if strings.HasSuffix(chunk, "\n") {
		return chunk[:len(chunk)-1], "\n"
	}

	return chunk, ""
}

func orDefault(v, fallback int) int {
	if v > 0 {
		return v
	}

	return fallback
}
