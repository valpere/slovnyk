// Package lookup implements bounded, dataset-pinned dictionary search. HTTP
// parsing and rendering are separate; errors expose only stable diagnostic codes.
package lookup

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/normalization"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

const (
	MaxResponseBytes = 1 << 20
	UIReserve        = 128 << 10
	EntryBudget      = MaxResponseBytes - UIReserve
	DBTimeout        = 2 * time.Second
)

type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string { return e.Code }

var (
	ErrInput       = &Error{"invalid_search_parameters", 400}
	ErrLocale      = &Error{"unknown_locale", 400}
	ErrPair        = &Error{"unavailable_pair", 404}
	ErrGone        = &Error{"cursor_dataset_unavailable", 410}
	ErrUnavailable = &Error{"lookup_unavailable", 503}
	ErrBudget      = &Error{"render_budget_exceeded", 503}
)

// Status maps a bounded application failure to its future HTTP status.
func Status(e error) int {
	if e == nil {
		return 200
	}
	if errors.Is(e, ErrCursor) {
		return 400
	}
	var known *Error
	if errors.As(e, &known) {
		return known.Status
	}
	return 503
}

type Request struct {
	From, To, Q string
	Mode        normalization.Mode
	Limit       int
	Cursor      string
}
type Group struct {
	Source       string
	Translations []string
}
type Page struct {
	Dataset string
	Entries []postgres.Translation
	Groups  []Group
	HasMore bool
	Cursor  string
	// EscapedBytes includes conservative per-entry markup, excluding UIReserve.
	EscapedBytes int
}
type Service struct{ signer *Signer }

func New(signer *Signer) (*Service, error) {
	if signer == nil {
		return nil, ErrCursor
	}
	return &Service{signer: signer}, nil
}

func validLocale(s string) bool {
	return len(s) > 0 && len(s) <= 64 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

// Search borrows a dedicated caller-owned connection. One deadline covers pin,
// catalog, anchor and search SQL. No transaction spans pages: ready content and
// publication history are immutable, and automatic retention/GC is disabled.
func (s *Service) Search(ctx context.Context, c *pgx.Conn, r Request) (Page, error) {
	ctx, cancel := context.WithTimeout(ctx, DBTimeout)
	defer cancel()
	out := Page{Entries: []postgres.Translation{}, Groups: []Group{}}
	if r.Mode == "" {
		r.Mode = normalization.Auto
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	if r.Mode != normalization.Auto && r.Mode != normalization.Exact && r.Mode != normalization.Mask || r.Limit < 1 || r.Limit > 100 {
		return out, ErrInput
	}
	trimmed, e := normalization.Validate(r.Q)
	if e != nil {
		return out, ErrInput
	}
	var token Cursor
	if r.Cursor != "" {
		token, e = s.signer.Decode(r.Cursor)
		if e != nil {
			return out, ErrCursor
		}
	}
	if trimmed == "" {
		if r.Cursor != "" {
			return out, ErrCursor
		}
		return out, nil
	}
	if !validLocale(r.From) || !validLocale(r.To) {
		return out, ErrInput
	}
	if c == nil {
		return out, ErrUnavailable
	}
	if r.Cursor != "" {
		out.Dataset = token.Dataset
	} else {
		if e = c.QueryRow(ctx, "SELECT coalesce(dataset_id::text,'') FROM slovnyk.active_dataset WHERE singleton=1").Scan(&out.Dataset); e != nil || out.Dataset == "" {
			return out, ErrUnavailable
		}
	}
	var state, version string
	var published bool
	e = c.QueryRow(ctx, `SELECT state,normalization_version,EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=d.id)
 FROM slovnyk.datasets d WHERE id=$1`, out.Dataset).Scan(&state, &version, &published)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) && r.Cursor != "" {
			return out, ErrGone
		}
		return out, ErrUnavailable
	}
	if state != "ready" || !published || version != normalization.Version {
		if r.Cursor != "" {
			return out, ErrGone
		}
		return out, ErrUnavailable
	}
	// Resolve the pinned version before compiling or checking its locales.
	query, e := normalization.Compile(r.Q, r.Mode)
	if e != nil {
		return out, ErrInput
	}
	fp := Fingerprint(version, r.From, r.To, string(query.Mode), query.Key, r.Limit)
	if r.Cursor != "" {
		decoded, _ := hex.DecodeString(token.Fingerprint)
		if token.Limit != r.Limit || !hmac.Equal(fp[:], decoded) {
			return out, ErrCursor
		}
	}
	var locales, pair bool
	e = c.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM slovnyk.dataset_locales WHERE dataset_id=$1 AND code=$2) AND
 EXISTS(SELECT 1 FROM slovnyk.dataset_locales WHERE dataset_id=$1 AND code=$3),
 EXISTS(SELECT 1 FROM slovnyk.dataset_pairs WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3)`, out.Dataset, r.From, r.To).Scan(&locales, &pair)
	if e != nil {
		return out, ErrUnavailable
	}
	if !locales {
		return out, ErrLocale
	}
	if !pair {
		return out, ErrPair
	}
	oversized := false
	out.HasMore, e = postgres.ReadPage(ctx, c, postgres.Filter{Dataset: out.Dataset, From: r.From, To: r.To, Query: query}, token.Anchor, r.Limit, func(t postgres.Translation) bool {
		n := EntryBytes(t)
		if n > EntryBudget {
			oversized = true
			return false
		}
		if out.EscapedBytes+n > EntryBudget {
			return false
		}
		out.Entries = append(out.Entries, t)
		out.EscapedBytes += n
		return true
	})
	if e != nil {
		if errors.Is(e, postgres.ErrAnchor) {
			return Page{}, ErrCursor
		}
		return Page{}, ErrUnavailable
	}
	if ctx.Err() != nil {
		return Page{}, ErrUnavailable
	}
	if oversized {
		return Page{}, ErrBudget
	}
	if out.HasMore {
		if len(out.Entries) == 0 {
			return Page{}, ErrBudget
		}
		out.Cursor, e = s.signer.Encode(Cursor{Dataset: out.Dataset, Fingerprint: hex.EncodeToString(fp[:]), Anchor: out.Entries[len(out.Entries)-1].ID, Limit: r.Limit})
		if e != nil {
			return Page{}, ErrUnavailable
		}
	}
	for _, t := range out.Entries {
		n := len(out.Groups)
		if n == 0 || out.Groups[n-1].Source != t.SourceText {
			out.Groups = append(out.Groups, Group{Source: t.SourceText, Translations: []string{t.TargetText}})
		} else {
			out.Groups[n-1].Translations = append(out.Groups[n-1].Translations, t.TargetText)
		}
	}
	return out, nil
}

// EntryBytes reserves an escaped source header, escaped translation and 64 bytes
// of per-entry HTML markup. M1-07's renderer must verify the actual final body;
// this conservative budget does not authorize truncation or oversized templates.
func EntryBytes(t postgres.Translation) int {
	return len(html.EscapeString(t.SourceText)) + len(html.EscapeString(t.TargetText)) + 64
}
