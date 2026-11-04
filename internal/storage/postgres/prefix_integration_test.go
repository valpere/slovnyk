//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/normalization"
)

func TestPrefixUpgradePreservesReadyData(t *testing.T) {
	c := database(t)
	ctx := context.Background()
	id := seed(t, c)
	raw := strings.Repeat("İ", 32768)
	key := normalization.Key(raw) // 98,304 bytes after full case folding.
	target := strings.Repeat("&", 65536)
	execSQL(t, c, translationSQL, id, 1, raw, target, key)
	// A selective fixture avoids asserting planner choice on a one-row pair.
	batch := &pgx.Batch{}
	for i := 0; i < 2048; i++ {
		text := fmt.Sprintf("z%06d", i)
		batch.Queue(translationSQL, id, i+2, text, "synthetic", text)
	}
	if err := c.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	ready(t, c, id)
	execSQL(t, c, `DROP INDEX slovnyk.translations_prefix64; DELETE FROM slovnyk.schema_migrations WHERE version=4`)
	if err := CheckSchema(ctx, c); !errors.Is(err, ErrSchema) {
		t.Fatal("schema 3 accepted", err)
	}
	if err := Migrate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, c); err != nil {
		t.Fatal("idempotence", err)
	}
	var gotRaw, gotTarget, gotKey, state string
	if err := c.QueryRow(ctx, `SELECT source_text,target_text,source_key,state FROM slovnyk.translations t JOIN slovnyk.datasets d ON d.id=t.dataset_id WHERE t.dataset_id=$1 AND t.id=1`, id).Scan(&gotRaw, &gotTarget, &gotKey, &state); err != nil {
		t.Fatal(err)
	}
	if gotRaw != raw || gotTarget != target || gotKey != key || state != "ready" {
		t.Fatal("upgrade changed ready data")
	}
	if err := CheckSchema(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Ensure the real expression index is usable under the HTTP role. Forcing
	// index eligibility here is not a claim about planner choice or performance.
	execSQL(t, c, `ANALYZE slovnyk.translations; SET ROLE slovnyk_http; SET enable_seqscan=off`)
	t.Cleanup(func() { execSQL(t, c, `RESET ROLE; RESET enable_seqscan`) })
	rows, err := c.Query(ctx, `EXPLAIN SELECT id FROM slovnyk.translations WHERE dataset_id=$1 AND source_locale='en' AND target_locale='uk' AND source_key LIKE $2 AND (left(source_key,64) COLLATE "C") LIKE $2`, id, "i%")
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	hasRange := false
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
		if strings.Contains(line, "Index Cond:") && strings.Contains(line, "left") {
			hasRange = true
		}
	}
	rows.Close()
	if rows.Err() != nil || !strings.Contains(plan.String(), `translations_prefix64`) || !hasRange {
		t.Fatalf("prefix index not eligible: %s; rows error: %v", plan.String(), rows.Err())
	}
}

func TestPrefixReadPageMatchesFullLike(t *testing.T) {
	c := database(t)
	ctx := context.Background()
	id := seed(t, c)
	long := strings.Repeat("😀", 64)
	raw := []string{"Straße", "STRASSE", "strasse", "Straße", "café", "cafe\u0301", "a%b", "axb", "a_b", `a\b`, " edge ", "İ", "i\u0307a", long + "a", long + "ab", long + "b", strings.Repeat("é", 63) + "%x", strings.Repeat("é", 63) + "_x", strings.Repeat("é", 63) + `\x`, strings.Repeat("İ", 32768)}
	for i, s := range raw {
		execSQL(t, c, translationSQL, id, int64(i+1), s, string(rune('A'+i)), normalization.Key(s))
	}
	ready(t, c, id)
	execSQL(t, c, `SET ROLE slovnyk_http`)
	t.Cleanup(func() { execSQL(t, c, `RESET ROLE`) })
	for _, mask := range []string{"str*", "str?ss?", "cafe\u0301*", "a%*", "a_*", `a\*`, "*b", "?*", "*", "*edge*", "İ*", long + "a*", long + "?", strings.Repeat("é", 63) + "%*", strings.Repeat("é", 63) + "_*", strings.Repeat("é", 63) + `\*`, "absent*"} {
		q, err := normalization.Compile(mask, normalization.Mask)
		if err != nil {
			t.Fatal(err)
		}
		// Reference is the original unoptimized full LIKE predicate and C order.
		rows, err := c.Query(ctx, `SELECT id FROM slovnyk.translations WHERE dataset_id=$1 AND source_locale='en' AND target_locale='uk' AND source_key LIKE $2 ESCAPE '\' ORDER BY source_key COLLATE "C",source_text COLLATE "C",target_text COLLATE "C",id`, id, q.Pattern)
		if err != nil {
			t.Fatal(err)
		}
		want := []int64{}
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			want = append(want, v)
		}
		rows.Close()
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		for _, limit := range []int{1, 2, 50} {
			got := []int64{}
			var anchor int64
			for page := 0; ; page++ {
				if page > len(raw) {
					t.Fatal("pagination did not terminate")
				}
				more, err := ReadPage(ctx, c, Filter{Dataset: id, From: "en", To: "uk", Query: q}, anchor, limit, func(v Translation) bool { got = append(got, v.ID); anchor = v.ID; return true })
				if err != nil {
					t.Fatal(err)
				}
				if !more {
					break
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("mask %q limit %d: got %v want %v", mask, limit, got, want)
			}
		}
	}
	q, _ := normalization.Compile(long+"a*", normalization.Mask)
	// Same 64-codepoint prefix, but full LIKE excludes the anchor's suffix.
	_, err := ReadPage(ctx, c, Filter{Dataset: id, From: "en", To: "uk", Query: q}, 16, 1, func(Translation) bool { return true })
	if !errors.Is(err, ErrAnchor) {
		t.Fatal("nonmatching anchor accepted", err)
	}
}
