package sitemaps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/artifacts"
)

const testDataset = "00000000-0000-0000-0000-000000000001"

func testStore(t *testing.T) (*artifacts.Store, string) {
	t.Helper()
	if err := os.MkdirAll("../../tmp/sitemaps", 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("../../tmp/sitemaps", "unit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	store, err := artifacts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, root
}
func sequence(n int) nextID {
	i := 0
	return func() (string, bool, error) {
		if i == n {
			return "", false, nil
		}
		i++
		return fmt.Sprintf("%064x", i), true, nil
	}
}
func artifactBytes(t *testing.T, store *artifacts.Store, ref artifacts.Ref) []byte {
	t.Helper()
	if err := store.Check(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	f, err := store.Reader(ref)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != ref.SHA256 || int64(len(b)) != ref.Bytes || ref.Bytes > maxBytes {
		t.Fatal("invalid artifact identity/size")
	}
	return b
}
func TestShardBoundariesAndDeterminism(t *testing.T) {
	for _, n := range []int{1, 50000, 50001} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			store, root := testStore(t)
			result, err := generate(context.Background(), store, testDataset, sequence(n), maxURLs, maxShards)
			if err != nil {
				t.Fatal(err)
			}
			if result.URLs != int64(n) || len(result.Shards) != (n+maxURLs-1)/maxURLs {
				t.Fatalf("wrong counts: %+v", result)
			}
			var actual []string
			for _, shard := range result.Shards {
				var doc struct {
					XMLName xml.Name
					URLs    []struct {
						Loc string `xml:"loc"`
					} `xml:"url"`
				}
				if err := xml.Unmarshal(artifactBytes(t, store, shard.Artifact), &doc); err != nil {
					t.Fatal(err)
				}
				if doc.XMLName != (xml.Name{Space: namespace, Local: "urlset"}) || len(doc.URLs) != shard.URLs || shard.URLs > maxURLs {
					t.Fatal("invalid shard")
				}
				for _, u := range doc.URLs {
					actual = append(actual, u.Loc)
				}
			}
			for i, loc := range actual {
				if loc != fmt.Sprintf("https://slovnyk.org/word/%064x", i+1) {
					t.Fatalf("wrong canonical URL %q", loc)
				}
			}
			var index struct {
				XMLName  xml.Name
				Sitemaps []struct {
					Loc string `xml:"loc"`
				} `xml:"sitemap"`
			}
			if err := xml.Unmarshal(artifactBytes(t, store, result.Index), &index); err != nil {
				t.Fatal(err)
			}
			if index.XMLName != (xml.Name{Space: namespace, Local: "sitemapindex"}) || len(index.Sitemaps) != len(result.Shards) {
				t.Fatal("invalid index")
			}
			for i, s := range index.Sitemaps {
				if s.Loc != origin+"/sitemap-"+testDataset+"-"+result.Shards[i].Artifact.SHA256+".xml" {
					t.Fatal("invalid discovery URL")
				}
			}
			var manifest Manifest
			if err := json.Unmarshal(artifactBytes(t, store, result.Report), &manifest); err != nil || !reflect.DeepEqual(manifest, result.Manifest) {
				t.Fatalf("invalid durable manifest: %v", err)
			}
			inode, err := os.Stat(filepath.Join(root, result.Report.Path))
			if err != nil {
				t.Fatal(err)
			}
			repeat, err := generate(context.Background(), store, testDataset, sequence(n), maxURLs, maxShards)
			if err != nil || !reflect.DeepEqual(repeat, result) {
				t.Fatalf("non-deterministic retry: %v", err)
			}
			after, err := os.Stat(filepath.Join(root, result.Report.Path))
			if err != nil || !os.SameFile(inode, after) {
				t.Fatal("retry replaced report")
			}
		})
	}
}
func TestIncompleteStreamsNeverCommitReport(t *testing.T) {
	for _, name := range []string{"empty", "invalid", "duplicate", "decreasing", "iterator failure", "cancelled", "shard limit"} {
		t.Run(name, func(t *testing.T) {
			store, root := testStore(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			next := sequence(5)
			want := ErrStorage
			limit := maxShards
			switch name {
			case "empty":
				next, want = sequence(0), ErrEmpty
			case "invalid":
				next = func() (string, bool, error) { return "<&secret>", true, nil }
			case "duplicate":
				next = func() (string, bool, error) { return strings.Repeat("a", 64), true, nil }
			case "decreasing":
				i := 5
				next = func() (string, bool, error) { i--; return fmt.Sprintf("%064x", i), true, nil }
			case "iterator failure":
				source := next
				i := 0
				next = func() (string, bool, error) {
					i++
					if i == 4 {
						return "", false, errors.New("private database detail")
					}
					return source()
				}
			case "cancelled":
				source := next
				i := 0
				next = func() (string, bool, error) {
					i++
					if i == 4 {
						cancel()
					}
					return source()
				}
			case "shard limit":
				limit, want = 1, ErrLimit
			}
			result, err := generate(ctx, store, testDataset, next, 2, limit)
			if !errors.Is(err, want) || !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("got %+v %v; want %v", result, err, want)
			}
			err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if strings.HasSuffix(path, ".json") || strings.HasPrefix(entry.Name(), ".pending-") {
					t.Fatalf("incomplete run committed report/temp: %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCorruptExistingShardFailsClosed(t *testing.T) {
	store, root := testStore(t)
	first, err := generate(context.Background(), store, testDataset, sequence(3), maxURLs, maxShards)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, first.Shards[0].Artifact.Path)
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := generate(context.Background(), store, testDataset, sequence(3), maxURLs, maxShards); err != ErrStorage || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("corrupt existing file accepted: %+v %v", result, err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "corrupt" {
		t.Fatal("existing artifact overwritten")
	}
}
