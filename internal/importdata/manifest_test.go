package importdata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/normalization"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/import/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestManifestGolden(t *testing.T) {
	m, err := ParseManifest(fixture(t, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, hash, err := m.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, fixture(t, "manifest.canonical.json")) || hex.EncodeToString(hash[:]) != strings.TrimSpace(string(fixture(t, "manifest.sha256"))) {
		t.Fatal("canonical manifest golden mismatch")
	}
	// Array and object order/whitespace are irrelevant; semantic edits are not.
	m.Locales[0], m.Locales[3] = m.Locales[3], m.Locales[0]
	m.ExpectedPairs[0], m.ExpectedPairs[3] = m.ExpectedPairs[3], m.ExpectedPairs[0]
	_, reordered, err := m.Canonical()
	if err != nil || reordered != hash {
		t.Fatal("order changed fingerprint", err)
	}
	m.Source.Authority = "other synthetic owner"
	_, changed, err := m.Canonical()
	if err != nil || changed == hash {
		t.Fatal("source edit not fingerprinted", err)
	}
}

func TestTupleAndKeyGolden(t *testing.T) {
	tuple := Tuple{"uk-ua", "en-us", "Слово", "word"}
	var b bytes.Buffer
	for _, s := range tuple {
		// Independent literal representation is stored in tuple.hex.
		n := uint64(len(s))
		for shift := 56; shift >= 0; shift -= 8 {
			b.WriteByte(byte(n >> shift))
		}
		b.WriteString(s)
	}
	if hex.EncodeToString(b.Bytes()) != strings.TrimSpace(string(fixture(t, "tuple.hex"))) {
		t.Fatal("tuple bytes mismatch")
	}
	h := TupleHash(tuple)
	if hex.EncodeToString(h[:]) != strings.TrimSpace(string(fixture(t, "tuple.sha256"))) {
		t.Fatal("tuple digest mismatch")
	}
	key := normalization.Key("Straße")
	kh := normalization.KeyHash(key)
	if hex.EncodeToString(kh[:]) != strings.TrimSpace(string(fixture(t, "key.sha256"))) {
		t.Fatal("key digest mismatch")
	}
	if TupleHash(Tuple{"a", "bc", "d", "e"}) == TupleHash(Tuple{"ab", "c", "d", "e"}) {
		t.Fatal("length boundaries lost")
	}
}

func TestManifestRejects(t *testing.T) {
	raw := string(fixture(t, "manifest.json"))
	for _, bad := range []string{
		strings.Replace(raw, `"schema_version": 1`, `"schema_version": 1, "schema_version": 1`, 1),
		strings.Replace(raw, `"kind":`, `"kind":"csv_snapshot", "kind":`, 1),
		strings.Replace(raw, `"schema_version": 1`, `"schema_version": 1.0`, 1),
		strings.Replace(raw, `"schema_version": 1`, `"schema_version": 1e0`, 1),
		strings.Replace(raw, `"schema_version": 1`, `"schema_version": 2`, 1),
		strings.Replace(raw, `"kind":`, `"unknown":true,"kind":`, 1),
		strings.Replace(raw, `"multi_pair": true`, `"multi_pair": null`, 1),
		strings.Replace(raw, `"pair": null,`, ``, 1),
		strings.Replace(raw, `"authority": "synthetic"`, `"authority": "\ud800"`, 1),
		strings.Replace(raw, `"authority": "synthetic"`, `"authority": "\udfff"`, 1),
		strings.Replace(raw, `"authority": "synthetic"`, `"authority": null`, 1),
		raw + " {}", raw + "\xff",
	} {
		if _, err := ParseManifest([]byte(bad)); err == nil {
			t.Errorf("accepted malformed manifest: %.80s", bad)
		}
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Artifacts = append(m.Artifacts, m.Artifacts[0]) },
		func(m *Manifest) { m.Artifacts[0].Name = "../outside.gz" },
		func(m *Manifest) { m.Artifacts[0].Name = "/outside.gz" },
		func(m *Manifest) { m.Artifacts[0].Name = "a/./b.gz" },
		func(m *Manifest) { m.Artifacts[0].Name = "a\\b.gz" },
		func(m *Manifest) { m.Artifacts[0].Format = "unknown" },
		func(m *Manifest) { m.Artifacts[0].SHA256 = strings.Repeat("A", 64) },
		func(m *Manifest) { m.Artifacts[0].Pair = &Pair{"uk-ua", "en-us"} },
		func(m *Manifest) { m.ExpectedPairs[0].To = "unknown" },
		func(m *Manifest) { m.AdmissionLimits.FieldBytes++ },
		func(m *Manifest) { m.Locales = append(m.Locales, m.Locales[0]) },
		func(m *Manifest) { m.MappingVersion = "merge-sr-v1" },
		func(m *Manifest) { m.NormalizationVersion = "unknown" },
		func(m *Manifest) { m.Source.SnapshotDate = "2026-02-30" },
		func(m *Manifest) { m.Exceptions = []Exception{{m.Artifacts[0].SHA256, 1, "bad", "approve_all"}} },
	} {
		m, _ := ParseManifest([]byte(raw))
		mutate(&m)
		if _, _, err := m.Canonical(); err == nil {
			t.Fatal("invalid manifest fingerprinted")
		}
	}
	// Valid surrogate pairs are decoded without changing their Unicode value.
	valid := strings.Replace(raw, `"authority": "synthetic"`, `"authority": "\ud83d\ude00"`, 1)
	if m, err := ParseManifest([]byte(valid)); err != nil || m.Source.Authority != "😀" {
		t.Fatal(m.Source, err)
	}
}

func TestArtifactAndSyntheticReconciliation(t *testing.T) {
	m, err := ParseManifest(fixture(t, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := OpenArtifact("../../testdata/import", m.Artifacts[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	compressed := io.TeeReader(f, h)
	r, err := NewGzipReader(compressed)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Locales = map[string]bool{}
	r.Pairs = map[Pair]bool{}
	for _, l := range m.Locales {
		r.Locales[l] = true
	}
	for _, p := range m.ExpectedPairs {
		r.Pairs[p] = true
	}
	seen := map[Tuple]bool{}
	unique, duplicates := 0, 0
	for {
		rec, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil || rec.Reason != "" {
			t.Fatal(rec, err)
		}
		if seen[rec.Fields] {
			duplicates++
		} else {
			unique++
			seen[rec.Fields] = true
		}
	}
	if unique != 10 || duplicates != 1 {
		t.Fatal(unique, duplicates)
	}
	if hex.EncodeToString(h.Sum(nil)) != m.Artifacts[0].SHA256 {
		t.Fatal("compressed artifact mismatch")
	}
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "outside.gz"), filepath.Join(dir, "link.gz"))
	a := m.Artifacts[0]
	a.Name = "link.gz"
	if f, err := OpenArtifact(dir, a); err == nil {
		f.Close()
		t.Fatal("symlink accepted")
	}
	os.Mkdir(filepath.Join(dir, "nested"), 0700)
	os.Symlink("nested", filepath.Join(dir, "alias"))
	a.Name = "alias/file.gz"
	if f, err := OpenArtifact(dir, a); err == nil {
		f.Close()
		t.Fatal("intermediate symlink accepted")
	}
	a = m.Artifacts[0]
	a.Bytes++
	if f, err := OpenArtifact("../../testdata/import", a); err == nil {
		f.Close()
		t.Fatal("wrong size accepted")
	}
}

func TestInvalidFixtures(t *testing.T) {
	var expected map[string]string
	if err := json.Unmarshal(fixture(t, "invalid/expected.json"), &expected); err != nil {
		t.Fatal(err)
	}
	for name, reason := range expected {
		rec, err := NewReader(bytes.NewReader(fixture(t, "invalid/"+name))).Next()
		if err != nil {
			if err.Error() != reason {
				t.Fatal(name, err)
			}
		} else if rec.Reason != reason {
			t.Fatal(name, rec)
		}
	}
}

func FuzzManifest(f *testing.F) {
	b, err := os.ReadFile("../../testdata/import/manifest.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":1}`))
	f.Add([]byte(`{"x":{"x":[1,2,3]}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := ParseManifest(b)
		if err != nil {
			return
		}
		canonical, hash, err := m.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		again, err := ParseManifest(canonical)
		if err != nil {
			t.Fatal(err)
		}
		other, nextHash, err := again.Canonical()
		if err != nil || hash != nextHash || !bytes.Equal(canonical, other) {
			t.Fatal("unstable canonical manifest", err)
		}
	})
}
