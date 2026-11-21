// Package artifact stores bounded immutable static builds. It never executes or
// extracts uploaded code. Paths from clients never select a host filesystem root.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

const (
	MaxBytes     = 16 << 20
	MaxFileBytes = 8 << 20
	MaxFiles     = 512
	MaxBody      = 24 << 20
)

var Limits = map[string]int{"max_bytes": MaxBytes, "max_file_bytes": MaxFileBytes, "max_files": MaxFiles, "max_body_bytes": MaxBody, "max_source_bytes": SourceLimit}

type File struct {
	Path    string `json:"path"`
	Content []byte `json:"content_base64"`
}
type FileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
	Hash string `json:"sha256"`
}
type Build struct {
	Hash  string     `json:"content_hash"`
	Files []FileInfo `json:"files"`
	Size  int        `json:"size_bytes"`
	Entry string     `json:"entrypoint"`
}

func ValidPath(name string) bool {
	if len(name) == 0 || len(name) > 240 || path.Clean(name) != name || strings.ContainsAny(name, "\\:%?#") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || part == "" {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) || r > 126 {
				return false
			}
		}
	}
	return !strings.HasPrefix(name, "_lastapp/")
}

func Inspect(files []File, entry string) (Build, error) {
	b := Build{Entry: entry, Files: []FileInfo{}}
	if !ValidPath(entry) || path.Ext(entry) != ".html" || len(files) == 0 || len(files) > MaxFiles {
		return b, fmt.Errorf("invalid entrypoint or file count")
	}
	seen := map[string]bool{}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range files {
		if !ValidPath(f.Path) || seen[f.Path] || len(f.Content) > MaxFileBytes {
			return b, fmt.Errorf("invalid, duplicate or oversized file")
		}
		seen[f.Path] = true
		b.Size += len(f.Content)
		if b.Size > MaxBytes {
			return b, fmt.Errorf("build exceeds size limit")
		}
		h := sha256.Sum256(f.Content)
		b.Files = append(b.Files, FileInfo{f.Path, len(f.Content), hex.EncodeToString(h[:])})
	}
	if !seen[entry] {
		return b, fmt.Errorf("entrypoint missing")
	}
	encoded, _ := json.Marshal(b)
	h := sha256.Sum256(encoded)
	b.Hash = hex.EncodeToString(h[:])
	return b, nil
}

func Store(directory string, files []File, b Build) error {
	if directory == "" {
		return fmt.Errorf("artifact directory not configured")
	}
	if e := os.MkdirAll(directory, 0o700); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(directory, "upload-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for _, f := range files {
		dest := filepath.Join(stage, filepath.FromSlash(f.Path))
		if e = os.MkdirAll(filepath.Dir(dest), 0o700); e != nil {
			return e
		}
		if e = os.WriteFile(dest, f.Content, 0o600); e != nil {
			return e
		}
	}
	encoded, _ := json.Marshal(b)
	if e = os.WriteFile(filepath.Join(stage, ".manifest.json"), encoded, 0o600); e != nil {
		return e
	}
	dest := filepath.Join(directory, b.Hash)
	if e = os.Rename(stage, dest); e == nil {
		return nil
	}
	// A content-identical retry is safe; a corrupt existing directory is not.
	existing, err := os.ReadFile(filepath.Join(dest, ".manifest.json"))
	if err == nil && string(existing) == string(encoded) {
		return nil
	}
	return e
}

func Read(directory, hash, name string) (io.ReadCloser, error) {
	if len(hash) != 64 || !ValidPath(name) {
		return nil, os.ErrNotExist
	}
	if _, e := hex.DecodeString(hash); e != nil {
		return nil, e
	}
	root, e := os.OpenRoot(directory)
	if e != nil {
		return nil, e
	}
	defer root.Close()
	return root.Open(hash + "/" + name)
}

// Check detects lost/corrupt storage before a release starts changing backend
// configuration, and again before its static activation.
func Check(directory string, b Build) error {
	if len(b.Files) == 0 || len(b.Files) > MaxFiles || b.Size > MaxBytes {
		return fmt.Errorf("invalid stored build")
	}
	entry := false
	for _, f := range b.Files {
		if f.Path == b.Entry {
			entry = true
		}
		if f.Size > MaxFileBytes || f.Size < 0 {
			return fmt.Errorf("invalid stored file")
		}
		r, e := Read(directory, b.Hash, f.Path)
		if e != nil {
			return e
		}
		h := sha256.New()
		size, e := io.Copy(h, io.LimitReader(r, int64(MaxFileBytes)+1))
		r.Close()
		if e != nil {
			return e
		}
		if size != int64(f.Size) || hex.EncodeToString(h.Sum(nil)) != f.Hash {
			return fmt.Errorf("stored file differs from uploaded build")
		}
	}
	if !entry {
		return fmt.Errorf("entrypoint missing")
	}
	return nil
}
