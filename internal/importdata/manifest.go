package importdata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/valpere/slovnyk/internal/normalization"
)

const MaxManifestBytes = 4 << 20

type Pair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Artifact struct {
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Name      string `json:"name"`
	Format    string `json:"format"`
	Pair      *Pair  `json:"pair"`
	MultiPair bool   `json:"multi_pair"`
}

type Limits struct {
	FieldBytes    int64 `json:"field_bytes"`
	RecordBytes   int64 `json:"record_bytes"`
	ArtifactBytes int64 `json:"artifact_bytes"`
}

type Exception struct {
	ArtifactSHA256 string `json:"artifact_sha256"`
	Ordinal        int64  `json:"ordinal"` // zero only for an excluded whole artifact
	Reason         string `json:"reason"`
	Decision       string `json:"decision"`
}

type Source struct {
	Kind         string `json:"kind"`
	Authority    string `json:"authority"`
	SnapshotDate string `json:"snapshot_date"`
	RightsStatus string `json:"rights_status"`
}

type Manifest struct {
	SchemaVersion        int         `json:"schema_version"`
	Source               Source      `json:"source"`
	Artifacts            []Artifact  `json:"artifacts"`
	Locales              []string    `json:"locales"`
	ExpectedPairs        []Pair      `json:"expected_pairs"`
	ParserVersion        string      `json:"parser_version"`
	MappingVersion       string      `json:"mapping_version"`
	NormalizationVersion string      `json:"normalization_version"`
	AdmissionLimits      Limits      `json:"admission_limits"`
	KnownGaps            []string    `json:"known_gaps"`
	Exceptions           []Exception `json:"exceptions"`
}

// ParseManifest rejects duplicate keys before typed decoding (including unknown
// nested keys), invalid UTF-8, surrogate escapes, floats, and trailing JSON.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return m, errors.New("invalid_manifest_size_or_encoding")
	}
	if err := validateSurrogates(data); err != nil {
		return m, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if _, err := strictValue(d, 0); err != nil {
		return m, err
	}
	if _, err := d.Token(); err != io.EOF {
		return m, errors.New("trailing_manifest_data")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	// Require all declared fields, including false/null/empty arrays, at every
	// level: comparing shapes also catches omitted fields with useful defaults.
	var actual, expected any
	json.Unmarshal(data, &actual)
	b, _ := json.Marshal(m)
	json.Unmarshal(b, &expected)
	if !sameShape(actual, expected) {
		return m, errors.New("missing_or_null_manifest_field")
	}
	return m, m.Validate()
}

func sameShape(a, b any) bool {
	switch expected := b.(type) {
	case map[string]any:
		actual, ok := a.(map[string]any)
		if !ok || len(actual) != len(expected) {
			return false
		}
		for k, v := range expected {
			if !sameShape(actual[k], v) {
				return false
			}
		}
	case []any:
		actual, ok := a.([]any)
		if !ok || len(actual) != len(expected) {
			return false
		}
		for i := range expected {
			if !sameShape(actual[i], expected[i]) {
				return false
			}
		}
	default:
		if a == nil && b != nil {
			return false
		}
	}
	return true
}

func strictValue(d *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("manifest_depth")
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		if x == '{' {
			m := map[string]any{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				s, ok := k.(string)
				if !ok {
					return nil, errors.New("invalid_json_key")
				}
				if _, ok := m[s]; ok {
					return nil, errors.New("duplicate_json_key")
				}
				v, err := strictValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				m[s] = v
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return m, nil
		}
		if x == '[' {
			a := []any{}
			for d.More() {
				v, err := strictValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return a, nil
		}
		return nil, errors.New("unexpected_json_delimiter")
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			return nil, errors.New("manifest_float")
		}
		if _, err := strconv.ParseInt(string(x), 10, 64); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// JSON syntax is validated by Decoder; this adds strict UTF-16 escape handling.
func validateSurrogates(b []byte) error {
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) || b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return errors.New("invalid_unicode_escape")
		}
		n, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return err
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return errors.New("unpaired_surrogate")
		}
		if n < 0xd800 || n > 0xdbff {
			continue
		}
		if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
			return errors.New("unpaired_surrogate")
		}
		low, err := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return errors.New("unpaired_surrogate")
		}
		i += 6
	}
	return nil
}

func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}

func validLocale(s string) bool {
	return len(s) > 0 && len(s) <= 64 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

func validText(s string) bool { return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

func validName(s string) bool {
	if !validText(s) || s == "." || path.IsAbs(s) || path.Clean(s) != s || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func (m Manifest) Validate() error {
	bad := func(s string) error { return fmt.Errorf("invalid_manifest: %s", s) }
	if m.SchemaVersion != 1 || m.ParserVersion != ParserVersion || m.MappingVersion != MappingVersion || m.NormalizationVersion != normalization.Version {
		return bad("version")
	}
	if m.Source.Kind != "csv_snapshot" || !validText(m.Source.Authority) || (m.Source.RightsStatus != "owner_confirmed" && m.Source.RightsStatus != "unknown") {
		return bad("source")
	}
	if m.Source.SnapshotDate != "unknown" {
		if _, err := time.Parse("2006-01-02", m.Source.SnapshotDate); err != nil {
			return bad("snapshot_date")
		}
	}
	if m.AdmissionLimits != (Limits{MaxFieldBytes, MaxRecordBytes, MaxArtifactBytes}) {
		return bad("admission_limits")
	}
	if len(m.Locales) == 0 || len(m.ExpectedPairs) == 0 || len(m.Artifacts) == 0 || m.KnownGaps == nil || m.Exceptions == nil {
		return bad("required_arrays")
	}
	locales, pairs := map[string]bool{}, map[Pair]bool{}
	for _, l := range m.Locales {
		if !validLocale(l) || locales[l] {
			return bad("locale")
		}
		locales[l] = true
	}
	for _, p := range m.ExpectedPairs {
		if !locales[p.From] || !locales[p.To] || pairs[p] {
			return bad("pair")
		}
		pairs[p] = true
	}
	hashes, names := map[string]bool{}, map[string]bool{}
	for _, a := range m.Artifacts {
		if !validHash(a.SHA256) || hashes[a.SHA256] || !validName(a.Name) || names[a.Name] || a.Bytes <= 0 || a.Format != "csv4.gz" || (a.Pair == nil) != a.MultiPair {
			return bad("artifact")
		}
		if a.Pair != nil && !pairs[*a.Pair] {
			return bad("artifact_pair")
		}
		hashes[a.SHA256], names[a.Name] = true, true
	}
	seen := map[string]bool{}
	for _, gap := range m.KnownGaps {
		if !validText(gap) {
			return bad("known_gap")
		}
	}
	for _, e := range m.Exceptions {
		if !validHash(e.ArtifactSHA256) || !validText(e.Reason) || (e.Decision != "exclude_record" && e.Decision != "exclude_artifact") || (e.Decision == "exclude_record" && (e.Ordinal < 1 || !hashes[e.ArtifactSHA256])) || (e.Decision == "exclude_artifact" && (e.Ordinal != 0 || hashes[e.ArtifactSHA256])) {
			return bad("exception")
		}
		key := e.ArtifactSHA256 + ":" + strconv.FormatInt(e.Ordinal, 10)
		if seen[key] {
			return bad("duplicate_exception")
		}
		seen[key] = true
	}
	return nil
}

// canonicalJSON uses encoding/json's string escapes (HTML escaping disabled),
// lexical object-key order, decimal integers, no insignificant whitespace/LF.
func canonicalJSON(v any) []byte {
	// Struct declaration order is not canonical object-key order. Convert to
	// JSON-only maps with integers retained before using the sorted map encoder.
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var generic any
	if err := d.Decode(&generic); err != nil {
		panic(err)
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(generic); err != nil {
		panic(err)
	} // internal JSON-only values
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

func (m Manifest) Canonical() ([]byte, [32]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, [32]byte{}, err
	}
	// Sort copies: fingerprinting must not mutate the operator's inventory.
	m.Artifacts = append([]Artifact{}, m.Artifacts...)
	m.Locales = append([]string{}, m.Locales...)
	m.ExpectedPairs = append([]Pair{}, m.ExpectedPairs...)
	m.KnownGaps = append([]string{}, m.KnownGaps...)
	m.Exceptions = append([]Exception{}, m.Exceptions...)
	sort.Slice(m.Artifacts, func(i, j int) bool { return m.Artifacts[i].SHA256 < m.Artifacts[j].SHA256 })
	sort.Slice(m.Locales, func(i, j int) bool {
		return bytes.Compare(canonicalJSON(m.Locales[i]), canonicalJSON(m.Locales[j])) < 0
	})
	sort.Slice(m.ExpectedPairs, func(i, j int) bool {
		return bytes.Compare(canonicalJSON(m.ExpectedPairs[i]), canonicalJSON(m.ExpectedPairs[j])) < 0
	})
	sort.Slice(m.KnownGaps, func(i, j int) bool {
		return bytes.Compare(canonicalJSON(m.KnownGaps[i]), canonicalJSON(m.KnownGaps[j])) < 0
	})
	sort.Slice(m.Exceptions, func(i, j int) bool {
		return bytes.Compare(canonicalJSON(m.Exceptions[i]), canonicalJSON(m.Exceptions[j])) < 0
	})
	// Marshal structs to generic objects so all keys, including nested ones, sort.
	d := json.NewDecoder(bytes.NewReader(canonicalJSON(m)))
	d.UseNumber()
	var object any
	if err := d.Decode(&object); err != nil {
		return nil, [32]byte{}, err
	}
	b := canonicalJSON(object)
	return b, sha256.Sum256(b), nil
}

// OpenArtifact confines access to root even if an immutable snapshot was
// misconfigured. Symlinks are refused in every path component before opening.
// The caller must keep the snapshot immutable throughout inspect/import/verify.
func OpenArtifact(root string, a Artifact) (*os.File, error) {
	if !validName(a.Name) {
		return nil, errors.New("invalid_artifact_path")
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	parts := strings.Split(a.Name, "/")
	for i := range parts {
		info, err := r.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("artifact_symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("artifact_not_directory")
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, errors.New("artifact_not_regular")
		}
	}
	f, err := r.Open(a.Name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != a.Bytes {
		f.Close()
		return nil, errors.New("artifact_size_or_type")
	}
	return f, nil
}
