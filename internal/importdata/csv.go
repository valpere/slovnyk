package importdata

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	MaxFieldBytes          = 64 << 10
	MaxRecordBytes         = 1 << 20
	MaxArtifactBytes int64 = 16 << 30
)

var (
	ErrSyntax        = errors.New("csv_syntax")
	ErrRecordLimit   = errors.New("record_limit")
	ErrArtifactLimit = errors.New("artifact_limit")
)

type Record struct {
	Ordinal int64
	Offset  int64 // decompressed byte offset, including the optional file BOM
	Fields  Tuple
	Reason  string // admission rejection; syntax/stream failures are returned errors
}

// Reader returns logical records, including blank lines. Any fatal error is
// sticky: a caller cannot resume beyond an unaccounted or malformed record.
type Reader struct {
	in              *bufio.Reader
	gz              *gzip.Reader
	ordinal, offset int64
	err             error
	started         bool
	Locales         map[string]bool
	Pairs           map[Pair]bool
}

func NewReader(r io.Reader) *Reader { return &Reader{in: bufio.NewReader(r)} }

// NewGzipReader accepts concatenated gzip members. Call Next until io.EOF;
// Close alone never proves checksum, trailer, or trailing-byte validity.
func NewGzipReader(r io.Reader) (*Reader, error) {
	g, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	p := NewReader(g)
	p.gz = g
	return p, nil
}

func (p *Reader) Close() error {
	if p.gz != nil {
		return p.gz.Close()
	}
	return nil
}

func (p *Reader) Next() (rec Record, err error) {
	if p.err != nil {
		return rec, p.err
	}
	defer func() {
		if err != nil {
			p.err = err
		}
	}()
	if !p.started {
		p.started = true
		if b, _ := p.in.Peek(3); string(b) == "\xef\xbb\xbf" {
			p.in.Discard(3)
			p.offset += 3
		}
	}
	rec.Offset = p.offset
	var field strings.Builder
	columns, state, count := 0, byte(0), 0 // 0=start, 1=bare, 2=quoted, 3=closed
	oversized := false
	appendByte := func(b byte) {
		if field.Len() <= MaxFieldBytes {
			field.WriteByte(b)
		} else {
			oversized = true
		}
	}
	finishField := func() {
		if field.Len() > MaxFieldBytes {
			oversized = true
		}
		if columns < 4 {
			rec.Fields[columns] = field.String()
		}
		columns++
		field.Reset()
	}
	read := func() (byte, error) {
		b, e := p.in.ReadByte()
		if e != nil {
			return 0, e
		}
		p.offset++
		count++
		if p.offset > MaxArtifactBytes {
			return 0, ErrArtifactLimit
		}
		if count > MaxRecordBytes {
			return 0, ErrRecordLimit
		}
		return b, nil
	}
	for {
		b, e := read()
		if e != nil {
			if e != io.EOF {
				return rec, e
			}
			if count == 0 {
				return rec, io.EOF
			}
			if state == 2 {
				return rec, ErrSyntax
			}
			finishField()
			break
		}
		if state == 2 {
			if b == '"' {
				state = 3
			} else {
				appendByte(b)
			}
			continue
		}
		if state == 3 && b == '"' {
			appendByte(b)
			state = 2
			continue
		}
		if b == ',' {
			finishField()
			state = 0
			continue
		}
		if b == '\n' || b == '\r' {
			if b == '\r' {
				next, e := read()
				if e != nil && e != io.EOF {
					return rec, e
				}
				if e != nil || next != '\n' {
					return rec, ErrSyntax
				}
			}
			finishField()
			break
		}
		if b == '"' {
			if state != 0 {
				return rec, ErrSyntax
			}
			state = 2
			continue
		}
		if state == 3 {
			return rec, ErrSyntax
		}
		state = 1
		appendByte(b)
	}
	p.ordinal++
	rec.Ordinal = p.ordinal
	switch {
	case columns != 4:
		rec.Reason = "field_count"
	case oversized:
		rec.Reason = "field_limit"
	default:
		for i, f := range rec.Fields {
			switch {
			case f == "":
				rec.Reason = "empty_field"
			case !utf8.ValidString(f):
				rec.Reason = "invalid_utf8"
			case strings.ContainsRune(f, 0):
				rec.Reason = "nul"
			case i < 2 && len(f) > 64:
				rec.Reason = "locale_limit"
			}
			if rec.Reason != "" {
				break
			}
		}
		if rec.Reason == "" && p.Locales != nil && (!p.Locales[rec.Fields[0]] || !p.Locales[rec.Fields[1]]) {
			rec.Reason = "unknown_locale"
		}
		if rec.Reason == "" && p.Pairs != nil && !p.Pairs[Pair{rec.Fields[0], rec.Fields[1]}] {
			rec.Reason = "unexpected_pair"
		}
	}
	return rec, nil
}

// WriteCSV4 preserves all admitted field bytes, including bare CR in quotes.
// It quotes every field and terminates each record with LF.
func WriteCSV4(w io.Writer, t Tuple) error {
	for i, field := range t {
		if i > 0 {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\""+strings.ReplaceAll(field, "\"", "\"\"")+"\""); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}
