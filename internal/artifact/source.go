package artifact

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SourceLimit applies to both stored and expanded source bytes.
const SourceLimit = 16 << 20

func sourceName(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == "node_modules" || part == ".git" || strings.HasPrefix(part, ".env") || part == "lastapp.local.json" {
			return false
		}
	}
	return ValidPath(name) || name == ".gitignore" || name == ".npmignore"
}

// StoreSource validates a portable source ZIP and stores it privately by digest.
func StoreSource(directory string, content []byte) (string, error) {
	if directory == "" || len(content) > SourceLimit {
		return "", fmt.Errorf("source archive storage is unavailable or upload exceeds its limit")
	}
	z, e := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if e != nil || len(z.File) == 0 || len(z.File) > MaxFiles {
		return "", fmt.Errorf("source requires a bounded ZIP file tree")
	}
	seen := map[string]bool{}
	regular := map[string]bool{}
	var total int64
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if seen[name] || !sourceName(name) || (f.Mode().Type() != 0 && !f.FileInfo().IsDir()) {
			return "", fmt.Errorf("invalid or duplicate source path")
		}
		seen[name] = true
		if f.FileInfo().IsDir() {
			continue
		}
		regular[name] = true
		r, e := f.Open()
		if e != nil {
			return "", e
		}
		n, e := io.Copy(io.Discard, io.LimitReader(r, SourceLimit-total+1))
		r.Close()
		total += n
		if e != nil || total > SourceLimit || uint64(n) != f.UncompressedSize64 {
			return "", fmt.Errorf("invalid or oversized expanded source")
		}
	}
	if !regular["package.json"] || !regular["package-lock.json"] {
		return "", fmt.Errorf("source requires package.json and package-lock.json")
	}
	hash := SourceHash(content)
	root := filepath.Join(directory, "private-source")
	if e := os.MkdirAll(root, 0o700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(root, ".pending-")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(content)
	closeErr := f.Close()
	if e != nil {
		return "", e
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hash, os.Rename(f.Name(), filepath.Join(root, hash))
}

// OpenSource opens a source object whose release authorization was checked by
// delivery. It verifies the full digest before any bytes reach the caller.
func OpenSource(directory, hash string) (*os.File, error) {
	if b, e := hex.DecodeString(hash); e != nil || len(b) != sha256.Size {
		return nil, fmt.Errorf("invalid source digest")
	}
	f, e := os.Open(filepath.Join(directory, "private-source", hash))
	if e != nil {
		return nil, e
	}
	sum := sha256.New()
	n, e := io.Copy(sum, io.LimitReader(f, SourceLimit+1))
	if e == nil && n <= SourceLimit && hex.EncodeToString(sum.Sum(nil)) == hash {
		_, e = f.Seek(0, io.SeekStart)
		if e == nil {
			return f, nil
		}
	}
	f.Close()
	return nil, fmt.Errorf("source integrity check failed")
}

func SourceHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
