package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func fixture(rows ...string) []byte {
	b := bytes.NewBufferString("PGCOPY\n\xff\r\n\x00")
	b.Write(make([]byte, 8))
	for _, v := range rows {
		binary.Write(b, binary.BigEndian, int16(1))
		binary.Write(b, binary.BigEndian, int32(len(v)))
		b.WriteString(v)
	}
	binary.Write(b, binary.BigEndian, int16(-1))
	return b.Bytes()
}
func TestMultisetOracle(t *testing.T) {
	a, e := digest(bytes.NewReader(fixture("a", "b", "a")))
	if e != nil {
		t.Fatal(e)
	}
	b, e := digest(bytes.NewReader(fixture("a", "a", "b")))
	if e != nil || a != b || a.Rows != 3 {
		t.Fatal("order invariance failed")
	}
	c, _ := digest(bytes.NewReader(fixture("a", "b", "b")))
	if c == a {
		t.Fatal("changed row not detected")
	}
	d, _ := digest(bytes.NewReader(fixture("a", "b")))
	if d == a {
		t.Fatal("missing duplicate not detected")
	}
	truncated := fixture("abc")
	if _, e = digest(bytes.NewReader(truncated[:len(truncated)-4])); e == nil {
		t.Fatal("truncated stream accepted")
	}
}

func TestCopyNullAndInvalidFraming(t *testing.T) {
	null := bytes.NewBufferString("PGCOPY\n\xff\r\n\x00")
	null.Write(make([]byte, 8))
	binary.Write(null, binary.BigEndian, int16(1))
	binary.Write(null, binary.BigEndian, int32(-1))
	binary.Write(null, binary.BigEndian, int16(-1))
	a, err := digest(bytes.NewReader(null.Bytes()))
	if err != nil || a.Rows != 1 {
		t.Fatal("NULL field rejected", err)
	}
	b, err := digest(bytes.NewReader(fixture("")))
	if err != nil || a == b {
		t.Fatal("NULL and empty field conflated", err)
	}
	badHeader := append([]byte{}, fixture()...)
	badHeader[0] = 'x'
	badFieldCount := append([]byte{}, fixture()...)
	binary.BigEndian.PutUint16(badFieldCount[19:], 0xfffe)
	badLength := append([]byte{}, fixture("x")...)
	binary.BigEndian.PutUint32(badLength[21:], 0xfffffffe)
	for _, data := range [][]byte{badHeader, badFieldCount, badLength, append(fixture(), 0)} {
		if _, err := digest(bytes.NewReader(data)); err == nil {
			t.Fatal("invalid COPY framing accepted")
		}
	}
}
