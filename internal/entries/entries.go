// Package entries serves raw-spelling dictionary pages with stable identities.
package entries

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

const PageSize = 50

var ErrNotFound = &lookup.Error{Code: "entry_not_found", Status: 404}

// ID hashes a domain prefix and three length-prefixed raw UTF-8 fields.
func ID(from, to, source string) string {
	h := sha256.New()
	h.Write([]byte("slovnyk-entry-v1\x00"))
	for _, s := range []string{from, to, source} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ValidID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 32 && hex.EncodeToString(b) == id
}

func Path(id string) string { return "/word/" + id }

type Page struct {
	Dataset, From, To, Source string
	Translations              []string
	Cursor                    string
}

// Read borrows c. A bare URL reads active once; a signed continuation reads its
// published retained dataset. Neither normalized search nor trimming is used.
func Read(ctx context.Context, c *pgx.Conn, signer *lookup.Signer, id, cursor string) (Page, error) {
	var out Page
	if !ValidID(id) {
		return out, ErrNotFound
	}
	var token lookup.Cursor
	var err error
	if cursor != "" {
		token, err = signer.Decode(cursor)
		if err != nil {
			return out, lookup.ErrCursor
		}
		out.Dataset = token.Dataset
	}
	if c == nil || signer == nil {
		return out, lookup.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, lookup.DBTimeout)
	defer cancel()
	if cursor == "" {
		if err = c.QueryRow(ctx, `SELECT coalesce(dataset_id::text,'') FROM slovnyk.active_dataset WHERE singleton=1`).Scan(&out.Dataset); err != nil || out.Dataset == "" {
			return Page{}, lookup.ErrUnavailable
		}
	}
	var published bool
	err = c.QueryRow(ctx, `SELECT state='ready' AND EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=d.id) FROM slovnyk.datasets d WHERE id=$1`, out.Dataset).Scan(&published)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !published {
		if cursor != "" {
			return Page{}, lookup.ErrGone
		}
		return Page{}, lookup.ErrUnavailable
	}
	if err != nil {
		return Page{}, lookup.ErrUnavailable
	}
	hash, _ := hex.DecodeString(id)
	const filter = `dataset_id=$1 AND slovnyk.entry_id(source_locale,target_locale,source_text)=$2`
	err = c.QueryRow(ctx, `SELECT source_locale,target_locale,source_text FROM slovnyk.translations WHERE `+filter+` ORDER BY id LIMIT 1`, out.Dataset, hash).Scan(&out.From, &out.To, &out.Source)
	if errors.Is(err, pgx.ErrNoRows) {
		if cursor != "" {
			return Page{}, lookup.ErrCursor
		}
		return Page{}, ErrNotFound
	}
	if err != nil || ID(out.From, out.To, out.Source) != id {
		return Page{}, lookup.ErrUnavailable
	}
	fp := lookup.Fingerprint("entry-v1", out.From, out.To, "raw", id, PageSize)
	if cursor != "" {
		if token.Fingerprint != hex.EncodeToString(fp[:]) || token.Limit != PageSize {
			return Page{}, lookup.ErrCursor
		}
		var valid bool
		err = c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM slovnyk.translations WHERE `+filter+` AND id=$3)`, out.Dataset, hash, token.Anchor).Scan(&valid)
		if err != nil {
			return Page{}, lookup.ErrUnavailable
		}
		if !valid {
			return Page{}, lookup.ErrCursor
		}
	}
	rows, err := c.Query(ctx, `SELECT id,source_locale,target_locale,source_text,target_text FROM slovnyk.translations WHERE `+filter+` AND id>$3 ORDER BY id LIMIT $4`, out.Dataset, hash, token.Anchor, PageSize+1)
	if err != nil {
		return Page{}, lookup.ErrUnavailable
	}
	defer rows.Close()
	budget, last, more := 0, int64(0), false
	for rows.Next() {
		var t postgres.Translation
		var from, to string
		if err = rows.Scan(&t.ID, &from, &to, &t.SourceText, &t.TargetText); err != nil || from != out.From || to != out.To || t.SourceText != out.Source {
			return Page{}, lookup.ErrUnavailable
		}
		n := lookup.EntryBytes(t)
		if len(out.Translations) == PageSize || budget+n > lookup.EntryBudget {
			more = true
			break
		}
		out.Translations = append(out.Translations, t.TargetText)
		budget += n
		last = t.ID
	}
	if rows.Err() != nil || ctx.Err() != nil {
		return Page{}, lookup.ErrUnavailable
	}
	if more {
		if last == 0 {
			return Page{}, lookup.ErrBudget
		}
		out.Cursor, err = signer.Encode(lookup.Cursor{Dataset: out.Dataset, Fingerprint: hex.EncodeToString(fp[:]), Anchor: last, Limit: PageSize})
		if err != nil {
			return Page{}, lookup.ErrUnavailable
		}
	}
	return out, nil
}
