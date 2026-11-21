package delivery

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aantich/lastapp/internal/registry"
)

// LockArtifacts serializes linking and deleting the same bytes without
// holding a registry transaction or blocking unrelated artifacts.
func (s *Service) LockArtifacts(ctx context.Context, hashes ...string) (func(), error) {
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return nil, e
	}
	held := []string{}
	unlock := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		defer conn.Close()
		for _, name := range held {
			if _, e := conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", name); e != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				return
			}
		}
	}
	slices.Sort(hashes)
	for _, hash := range slices.Compact(hashes) {
		if hash == "" {
			continue
		}
		raw, e := hex.DecodeString(hash)
		if e != nil || len(raw) != 32 {
			unlock()
			return nil, registry.Bad("Invalid artifact digest.")
		}
		name := "lastapp:asset:" + hash[:32]
		var got sql.NullBool
		if e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,2)", name).Scan(&got); e != nil || !got.Bool {
			unlock()
			if e != nil {
				return nil, e
			}
			return nil, registry.Conflict("Artifact storage is busy. Retry the operation.")
		}
		held = append(held, name)
	}
	return unlock, nil
}

func (s *Service) PruneArtifacts(ctx context.Context) (int, error) {
	root := s.ArtifactDirectory()
	if root == "" {
		return 0, registry.Conflict("Configure persistent artifact storage first.")
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	removed := 0
	for _, source := range []bool{false, true} {
		dir := root
		if source {
			dir = filepath.Join(root, "private-source")
		}
		n, e := s.pruneDirectory(ctx, dir, source, cutoff)
		removed += n
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return removed, e
		}
	}
	return removed, nil
}

func (s *Service) pruneDirectory(ctx context.Context, dir string, source bool, cutoff time.Time) (int, error) {
	f, e := os.Open(dir)
	if e != nil {
		return 0, e
	}
	defer f.Close()
	removed := 0
	for {
		if e = ctx.Err(); e != nil {
			return removed, e
		}
		entries, readErr := f.ReadDir(64)
		for _, entry := range entries {
			n, e := s.pruneArtifact(ctx, dir, entry, source, cutoff)
			removed += n
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return removed, e
			}
		}
		if errors.Is(readErr, io.EOF) {
			return removed, nil
		}
		if readErr != nil {
			return removed, readErr
		}
	}
}

func (s *Service) pruneArtifact(ctx context.Context, dir string, entry os.DirEntry, source bool, cutoff time.Time) (int, error) {
	name := entry.Name()
	path := filepath.Join(dir, name)
	info, e := entry.Info()
	if e != nil || !info.ModTime().Before(cutoff) || info.Mode()&os.ModeSymlink != 0 {
		return 0, e
	}
	pending := source && strings.HasPrefix(name, ".pending-") && !entry.IsDir() || !source && strings.HasPrefix(name, "upload-") && entry.IsDir()
	if pending {
		return 1, os.RemoveAll(path)
	}
	key, e := hex.DecodeString(name)
	if e != nil || len(key) != 32 || entry.IsDir() == source {
		return 0, nil
	}
	if !source {
		if _, e = os.Stat(filepath.Join(path, ".manifest.json")); e != nil {
			return 0, e
		}
	}
	unlock, e := s.LockArtifacts(ctx, name)
	if e != nil {
		return 0, e
	}
	defer unlock()
	info, e = os.Lstat(path)
	if e != nil || !info.ModTime().Before(cutoff) {
		return 0, e
	}
	keep, e := artifactReferenced(ctx, s.DB, name, source)
	if e != nil || keep {
		return 0, e
	}
	if e = os.RemoveAll(path); e != nil {
		return 0, e
	}
	return 1, nil
}

func artifactReferenced(ctx context.Context, q registry.Queryer, hash string, source bool) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM app_builds WHERE content_hash=?) OR EXISTS(SELECT 1 FROM catalog_packages WHERE build_hash=?)`
	args := []any{hash, hash}
	if source {
		query = `SELECT EXISTS(SELECT 1 FROM build_sources WHERE content_hash=?) OR EXISTS(SELECT 1 FROM release_sources WHERE content_hash=?) OR EXISTS(SELECT 1 FROM catalog_packages WHERE source_hash=?)`
		args = append(args, hash)
	}
	var keep bool
	e := q.QueryRowContext(ctx, query, args...).Scan(&keep)
	return keep, e
}

// The upload transaction has rolled back and the digest locks are still held.
// Check references in case a committed transaction lost its acknowledgment.
func (s *Service) reclaimUpload(ctx context.Context, tenant, build string, storedBuild bool, source string, storedSource bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		s.Warn(ctx, "failed to check rolled-back upload", e)
		return
	}
	defer tx.Rollback()
	// Wait for an earlier upload transaction whose commit outcome was unknown.
	if e = registry.LockTenant(ctx, tx, tenant); e != nil {
		s.Warn(ctx, "failed to check rolled-back upload", e)
		return
	}
	remove := func(dir, hash string, source bool) {
		keep, e := artifactReferenced(ctx, tx, hash, source)
		if e == nil && !keep {
			e = os.RemoveAll(filepath.Join(dir, hash))
		}
		if e != nil {
			s.Warn(ctx, "failed to reclaim unreferenced upload", e)
		}
	}
	if storedBuild {
		remove(s.ArtifactDirectory(), build, false)
	}
	if storedSource {
		remove(filepath.Join(s.ArtifactDirectory(), "private-source"), source, true)
	}
}
