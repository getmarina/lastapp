package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/control"
	"github.com/aantich/lastapp/internal/registry"
)

func publishPackage(ctx context.Context, s *control.Server, manifest, build, source, entry string) error {
	input, e := os.Open(manifest)
	if e != nil {
		return e
	}
	defer input.Close()
	var files []artifact.File
	var compiled *artifact.Build
	var archive []byte
	if build != "" {
		size := 0
		e = filepath.WalkDir(build, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			name, e := filepath.Rel(build, path)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || len(files) >= artifact.MaxFiles || !artifact.ValidPath(filepath.ToSlash(name)) {
				return fmt.Errorf("invalid build file path")
			}
			b, e := readBounded(path, artifact.MaxFileBytes)
			if e != nil {
				return e
			}
			size += len(b)
			if size > artifact.MaxBytes {
				return fmt.Errorf("build exceeds byte limit")
			}
			files = append(files, artifact.File{Path: filepath.ToSlash(name), Content: b})
			return nil
		})
		if e != nil {
			return e
		}
		b, e := artifact.Inspect(files, entry)
		if e != nil {
			return e
		}
		compiled = &b
	}
	if source != "" {
		archive, e = readBounded(source, artifact.SourceLimit)
		if e != nil {
			return e
		}
	}
	v, e := s.PublishPackage(ctx, input, compiled, files, archive)
	if e == nil {
		fmt.Printf("Published %s %s %s\n", v.Key, v.Version, v.Digest)
	}
	return e
}

func readBounded(path string, limit int) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if e == nil && len(b) > limit {
		e = registry.Bad("File exceeds its byte limit.")
	}
	return b, e
}
