package importdata

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type chunks struct {
	data []byte
	size int
}

func (r *chunks) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.size, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func TestCSVFixtures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/import/corpus.csv")
	if err != nil {
		t.Fatal(err)
	}
	want := []Tuple{
		{"uk-ua", "en-us", "Слово", "word"}, {"uk-ua", "en-us", "Слово", "word"},
		{"uk-ua", "en-us", "СЛОВО", "term"}, {"en-us", "uk-ua", "Straße", "вулиця"},
		{"sr-rs", "sr-rs", "реч", "реч"}, {"en-us", "uk-ua", "E\u0301", "акцент"},
		{"en-us", "uk-ua", " edge ", " краї "}, {"en-us", "uk-ua", "*?%_\\", "маска"},
		{"en-us", "uk-ua", "a,\"b", "CR\rLF\nCRLF\r\nend"},
		{"en-us", "uk-ua", "👩‍💻", "emoji"}, {"sr-sp", "sr-rs", "reč", "реч"},
	}
	for _, size := range []int{1, 2, 3, 7, 4096} {
		r := NewReader(&chunks{data, size})
		for i, w := range want {
			rec, err := r.Next()
			if err != nil || rec.Ordinal != int64(i+1) || rec.Reason != "" || rec.Fields != w {
				t.Fatalf("chunk=%d row=%d: %+v %v", size, i, rec, err)
			}
		}
		if _, err := r.Next(); err != io.EOF {
			t.Fatal(err)
		}
	}
}

func TestCSVAdmissionAndSyntax(t *testing.T) {
	for _, tc := range []struct {
		raw, reason string
		fatal       error
	}{
		{"\n", "field_count", nil}, {"a,b,c\n", "field_count", nil}, {"a,b,c,d,e\n", "field_count", nil},
		{"a,b,,d\n", "empty_field", nil}, {"a,b,c,\xff\n", "invalid_utf8", nil}, {"a,b,c,d\x00\n", "nul", nil},
		{"a,b," + strings.Repeat("x", MaxFieldBytes+1) + ",d\n", "field_limit", nil},
		{strings.Repeat("a", 65) + ",b,c,d\n", "locale_limit", nil},
		{"a,b,\"c\"x,d\n", "", ErrSyntax}, {"a,b,c,d\r", "", ErrSyntax}, {"a,b,c\"d,e\n", "", ErrSyntax},
		{"a,b,\"c,d", "", ErrSyntax}, {"a,b," + strings.Repeat("x", MaxRecordBytes) + ",d", "", ErrRecordLimit},
	} {
		r := NewReader(strings.NewReader(tc.raw))
		rec, err := r.Next()
		if !errors.Is(err, tc.fatal) || rec.Reason != tc.reason {
			t.Errorf("len=%d got %+v %v; want %s %v", len(tc.raw), rec, err, tc.reason, tc.fatal)
		}
		if err != nil {
			if _, next := r.Next(); !errors.Is(next, tc.fatal) {
				t.Fatal("fatal error not sticky", next)
			}
		}
	}
	r := NewReader(strings.NewReader("a,b,c,d\n"))
	r.Locales = map[string]bool{"a": true}
	if rec, _ := r.Next(); rec.Reason != "unknown_locale" {
		t.Fatal(rec)
	}
	r = NewReader(strings.NewReader("a,b,c,d\n"))
	r.Pairs = map[Pair]bool{{"b", "a"}: true}
	if rec, _ := r.Next(); rec.Reason != "unexpected_pair" {
		t.Fatal(rec)
	}
}

func zipBytes(raw []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(raw)
	w.Close()
	return b.Bytes()
}

func TestGzipEOF(t *testing.T) {
	first := zipBytes([]byte("a,b,c,d\n"))
	for _, tc := range []struct {
		name    string
		data    []byte
		rows    int
		success bool
	}{
		{"clean", first, 1, true},
		{"concatenated", append(append([]byte{}, first...), zipBytes([]byte("a,b,e,f"))...), 2, true},
		{"truncated", first[:len(first)-3], -1, false},
		{"trailing", append(append([]byte{}, first...), []byte("garbage trailing")...), -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewGzipReader(bytes.NewReader(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			rows := 0
			for {
				_, err = r.Next()
				if err != nil {
					break
				}
				rows++
			}
			if (err == io.EOF) != tc.success || (tc.rows >= 0 && rows != tc.rows) {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
		})
	}
	corrupt := append([]byte{}, first...)
	corrupt[len(corrupt)-8] ^= 1
	r, err := NewGzipReader(bytes.NewReader(corrupt))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for err == nil {
		_, err = r.Next()
	}
	if !errors.Is(err, gzip.ErrChecksum) {
		t.Fatal(err)
	}
}

func TestCSVWriteRoundTrip(t *testing.T) {
	want := Tuple{"sr-rs", "sr-rs", "\ufeffa,\"\r\nb\rc\n", " edge "}
	var b bytes.Buffer
	if err := WriteCSV4(&b, want); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&b)
	rec, err := r.Next()
	if err != nil || rec.Fields != want {
		t.Fatal(rec, err)
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatal(err)
	}
	if err := WriteCSV4(failingWriter{}, want); err == nil {
		t.Fatal("writer error lost")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestArtifactLimit(t *testing.T) {
	r := NewReader(strings.NewReader("a,b,c,d\n"))
	r.started = true
	r.offset = MaxArtifactBytes - 2
	if _, err := r.Next(); !errors.Is(err, ErrArtifactLimit) {
		t.Fatal(err)
	}
}

func TestCSVBoundaryAndRejectedOrdinals(t *testing.T) {
	text := strings.Repeat("x", MaxFieldBytes)
	r := NewReader(strings.NewReader("\na,b," + text + ",d\r\na,b," + text + "x,d\na,b,c,d"))
	for i, reason := range []string{"field_count", "", "field_limit", ""} {
		rec, err := r.Next()
		if err != nil || rec.Ordinal != int64(i+1) || rec.Reason != reason {
			t.Fatal(i, rec, err)
		}
		if i == 1 && rec.Fields[2] != text {
			t.Fatal("boundary field changed")
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatal(err)
	}
}

func FuzzCSV(f *testing.F) {
	for _, seed := range []string{"a,b,c,d\n", "a,b,\"c\r\nd\",e", "\n", "a,b,\"c"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > MaxRecordBytes+1 {
			t.Skip()
		}
		r := NewReader(strings.NewReader(raw))
		for {
			rec, err := r.Next()
			if err != nil {
				return
			}
			if rec.Reason != "" {
				continue
			}
			var b bytes.Buffer
			if err := WriteCSV4(&b, rec.Fields); err != nil {
				t.Fatal(err)
			}
			got, err := NewReader(&b).Next()
			if err != nil || got.Fields != rec.Fields || got.Reason != "" {
				t.Fatal("lossy round trip", err)
			}
		}
	})
}
