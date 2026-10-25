// Package artifacts installs immutable files on a local filesystem using a
// synced temporary inode, atomic no-clobber hard link and directory fsync.
package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

var ErrArtifact = errors.New("artifact_unavailable_or_invalid")

type Ref struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type Store struct {
	root  *os.Root
	fault func(string) error
}

// Open never creates anything and is safe for a read-only HTTP mount.
func Open(root string) (*Store, error) {
	r, e := os.OpenRoot(root)
	if e != nil {
		return nil, ErrArtifact
	}
	return &Store{root: r}, nil
}
func (s *Store) Close() error { return s.root.Close() }
func valid(p string) bool {
	return p != "" && p != "." && p != ".." && !path.IsAbs(p) && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00") && !strings.HasPrefix(p, "../")
}
func (s *Store) components(p string) error {
	if !valid(p) {
		return ErrArtifact
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		info, e := s.root.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return ErrArtifact
		}
		if (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return ErrArtifact
		}
	}
	return nil
}
func (s *Store) mkdir(p string) error {
	if !valid(p) {
		return ErrArtifact
	}
	current := ""
	for _, part := range strings.Split(p, "/") {
		current = path.Join(current, part)
		if e := s.root.Mkdir(current, 0750); e != nil && !errors.Is(e, os.ErrExist) {
			return ErrArtifact
		}
		info, e := s.root.Lstat(current)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrArtifact
		}
		if e = s.syncDir(path.Dir(current)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) syncDir(p string) error {
	f, e := s.root.Open(p)
	if e != nil {
		return ErrArtifact
	}
	defer f.Close()
	if e = f.Sync(); e != nil {
		return ErrArtifact
	}
	return nil
}
func (s *Store) hit(point string) error {
	if s.fault != nil {
		return s.fault(point)
	}
	return nil
}

// Install derives the basename from the completed content hash. Directory and
// suffix are service-generated identities, never arbitrary HTTP parameters.
func (s *Store) Install(ctx context.Context, dir, suffix string, write func(io.Writer) error) (Ref, error) {
	var ref Ref
	if !valid(dir) || (suffix != ".csv.gz" && suffix != ".json" && suffix != ".xml") {
		return ref, ErrArtifact
	}
	if e := s.mkdir(dir); e != nil {
		return ref, e
	}
	temp := path.Join(dir, ".pending-"+rand.Text())
	f, e := s.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ref, ErrArtifact
	}
	defer func() { _ = f.Close(); _ = s.root.Remove(temp) }()
	if e = write(contextWriter{ctx, f}); e != nil {
		return ref, e
	}
	if e = s.hit("written"); e != nil {
		return ref, e
	}
	if e = f.Chmod(0440); e != nil {
		return ref, ErrArtifact
	}
	if e = f.Sync(); e != nil {
		return ref, ErrArtifact
	}
	if e = s.hit("file_synced"); e != nil {
		return ref, e
	}
	info, e := f.Stat()
	if e != nil {
		return ref, ErrArtifact
	}
	if e = f.Close(); e != nil {
		return ref, ErrArtifact
	}
	h := sha256.New()
	read, e := s.root.Open(temp)
	if e != nil {
		return ref, ErrArtifact
	}
	_, e = io.Copy(h, contextReader{ctx, read})
	_ = read.Close()
	if e != nil {
		return ref, e
	}
	ref = Ref{Path: path.Join(dir, hex.EncodeToString(h.Sum(nil))+suffix), SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: info.Size()}
	if e = ctx.Err(); e != nil {
		return Ref{}, e
	}
	if e = s.root.Link(temp, ref.Path); e != nil {
		if !errors.Is(e, os.ErrExist) {
			return Ref{}, ErrArtifact
		}
		if e = s.Check(ctx, ref); e != nil {
			return Ref{}, e
		}
	}
	if e = s.hit("linked"); e != nil {
		return Ref{}, e
	}
	// Sync each installation even if a previous interrupted attempt created the
	// same final name without reaching its directory fsync.
	if e = s.syncDir(dir); e != nil {
		return Ref{}, e
	}
	if e = s.hit("directory_synced"); e != nil {
		return Ref{}, e
	}
	return ref, nil
}

func (s *Store) Reader(ref Ref) (*os.File, error) {
	if e := s.components(ref.Path); e != nil {
		return nil, e
	}
	f, e := s.root.Open(ref.Path)
	if e != nil {
		return nil, ErrArtifact
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != ref.Bytes {
		_ = f.Close()
		return nil, ErrArtifact
	}
	return f, nil
}
func (s *Store) Check(ctx context.Context, ref Ref) error {
	if len(ref.SHA256) != 64 {
		return ErrArtifact
	}
	expected, e := hex.DecodeString(ref.SHA256)
	if e != nil || hex.EncodeToString(expected) != ref.SHA256 {
		return ErrArtifact
	}
	f, e := s.Reader(ref)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, contextReader{ctx, f}); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != ref.SHA256 {
		return ErrArtifact
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(b []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	return w.w.Write(b)
}

// ReaderSize opens a confined regular file of bounded size, for metadata whose
// digest is stored in the database but whose length is not part of that schema.
func (s *Store) ReaderSize(p string, max int64) (*os.File, error) {
	if e := s.components(p); e != nil {
		return nil, e
	}
	f, e := s.root.Open(p)
	if e != nil {
		return nil, ErrArtifact
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > max {
		_ = f.Close()
		return nil, ErrArtifact
	}
	return f, nil
}
