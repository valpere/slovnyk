package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type result struct {
	Rows uint64 `json:"rows"`
	Sum  string `json:"sha256_sum_mod_2_256"`
	XOR  string `json:"sha256_xor"`
}

func digest(r io.Reader) (result, error) {
	b := bufio.NewReaderSize(r, 256*1024)
	header := make([]byte, 19)
	if _, e := io.ReadFull(b, header); e != nil {
		return result{}, e
	}
	if string(header[:11]) != "PGCOPY\n\xff\r\n\x00" || binary.BigEndian.Uint32(header[11:15]) != 0 {
		return result{}, fmt.Errorf("unsupported COPY header")
	}
	if _, e := io.CopyN(io.Discard, b, int64(binary.BigEndian.Uint32(header[15:]))); e != nil {
		return result{}, e
	}
	var count uint64
	var sum, xor [4]uint64
	scratch := make([]byte, 32*1024)
	for {
		var n [2]byte
		if _, e := io.ReadFull(b, n[:]); e != nil {
			return result{}, e
		}
		fields := int16(binary.BigEndian.Uint16(n[:]))
		if fields == -1 {
			break
		}
		if fields < 0 {
			return result{}, fmt.Errorf("invalid fields")
		}
		h := sha256.New()
		h.Write(n[:])
		for i := int16(0); i < fields; i++ {
			var size [4]byte
			if _, e := io.ReadFull(b, size[:]); e != nil {
				return result{}, e
			}
			h.Write(size[:])
			length := int32(binary.BigEndian.Uint32(size[:]))
			if length < -1 {
				return result{}, fmt.Errorf("invalid size")
			}
			if length >= 0 {
				if n, e := io.CopyBuffer(h, io.LimitReader(b, int64(length)), scratch); e != nil || n != int64(length) {
					return result{}, fmt.Errorf("truncated field")
				}
			}
		}
		hash := h.Sum(nil)
		var carry uint64
		for i := 3; i >= 0; i-- {
			v := binary.BigEndian.Uint64(hash[i*8 : (i+1)*8])
			sum[i], carry = bits.Add64(sum[i], v, carry)
			xor[i] ^= v
		}
		count++
	}
	if _, e := b.ReadByte(); e != io.EOF {
		return result{}, fmt.Errorf("COPY trailing data")
	}
	encode := func(v [4]uint64) string {
		var b [32]byte
		for i, x := range v {
			binary.BigEndian.PutUint64(b[i*8:], x)
		}
		return hex.EncodeToString(b[:])
	}
	return result{count, encode(sum), encode(xor)}, nil
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: inventory CREDENTIALS OUTPUT")
	}
	raw, e := os.ReadFile(os.Args[1])
	if e != nil {
		return e
	}
	var c map[string]string
	if e = json.Unmarshal(raw, &c); e != nil {
		return e
	}
	u := url.URL{Scheme: "postgresql", Host: c["address"], Path: "/" + c["db"], User: url.UserPassword(c["admin"], c["admin_password"]), RawQuery: "sslmode=disable"}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	defer cancel()
	conn, e := pgx.Connect(ctx, u.String())
	if e != nil {
		return fmt.Errorf("private connection failed")
	}
	defer conn.Close(ctx)
	if _, e = conn.Exec(ctx, "SET statement_timeout=0; SET timezone='UTC'; SET datestyle='ISO,YMD'"); e != nil {
		return e
	}
	q, e := conn.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname='slovnyk' ORDER BY tablename")
	if e != nil {
		return e
	}
	var tables []string
	for q.Next() {
		var s string
		if e = q.Scan(&s); e != nil {
			return e
		}
		tables = append(tables, s)
	}
	q.Close()
	if e = q.Err(); e != nil {
		return e
	}
	if len(tables) != 17 {
		return fmt.Errorf("unexpected table count")
	}
	out := map[string]result{}
	for _, table := range tables {
		started := time.Now()
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			_, e := conn.PgConn().CopyTo(ctx, pw, "COPY "+pgx.Identifier{"slovnyk", table}.Sanitize()+" TO STDOUT (FORMAT binary)")
			pw.CloseWithError(e)
			done <- e
		}()
		value, de := digest(pr)
		pr.CloseWithError(de)
		ce := <-done
		if de != nil || ce != nil {
			return fmt.Errorf("private table digest failed: %s", table)
		}
		out[table] = value
		fmt.Printf("%s rows=%d seconds=%.3f\n", table, value.Rows, time.Since(started).Seconds())
	}
	// Sequence state is preserved separately: it is not represented by table COPY.
	q, e = conn.Query(ctx, "SELECT sequencename FROM pg_sequences WHERE schemaname='slovnyk' ORDER BY sequencename")
	if e != nil {
		return e
	}
	var names []string
	for q.Next() {
		var s string
		if e = q.Scan(&s); e != nil {
			return e
		}
		names = append(names, s)
	}
	q.Close()
	if e = q.Err(); e != nil {
		return e
	}
	sequences := map[string]any{}
	for _, name := range names {
		var last int64
		var called bool
		if e = conn.QueryRow(ctx, "SELECT last_value,is_called FROM "+pgx.Identifier{"slovnyk", name}.Sanitize()).Scan(&last, &called); e != nil {
			return e
		}
		sequences[name] = map[string]any{"last_value": last, "is_called": called}
	}
	content, _ := json.MarshalIndent(map[string]any{"tables": out, "sequences": sequences, "algorithm": "binary-COPY-row SHA256, commutative sum mod2^256 and XOR; same built-in binary encodings; no raw rows retained"}, "", "  ")
	f, e := os.OpenFile(os.Args[2], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(append(content, '\n')); e != nil {
		return e
	}
	return f.Sync()
}
func main() {
	if e := run(); e != nil {
		if strings.Contains(e.Error(), "unexpected table count") {
			fmt.Fprintln(os.Stderr, e)
		} else {
			fmt.Fprintln(os.Stderr, "Inventory failed; private diagnostics withheld")
		}
		os.Exit(1)
	}
}
