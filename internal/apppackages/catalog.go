// Package apppackages owns the first-party package catalog, portable references,
// and tenant installation plans.
package apppackages

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/aantich/lastapp/internal/artifact"
	"github.com/aantich/lastapp/internal/delivery"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/op"
	"github.com/aantich/lastapp/internal/registry"
)

const (
	MaxManifest     = 256 << 10
	MaxItems        = 64
	MaxDependencies = 16
	Format          = 1
)

type Service struct {
	DB       *sql.DB
	Run      *op.Runner
	Delivery *delivery.Service
}

type Manifest struct {
	Requires     []string            `json:"requires,omitempty"`
	Format       int                 `json:"format"`
	Key          string              `json:"key"`
	Version      string              `json:"version"`
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Dependencies map[string]string   `json:"dependencies,omitempty"`
	Definitions  []entity.Definition `json:"entities"`
	Roles        []Role              `json:"roles"`
	App          *App                `json:"app,omitempty"`
}

type Role struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type App struct {
	LaunchRoles []string         `json:"launch_roles,omitempty"`
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	Access      entity.AppAccess `json:"access"`
	Roles       []Role           `json:"roles,omitempty"`
	Microapps   []Microapp       `json:"microapps"`
}

type Microapp struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Version struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Name    string `json:"name"`
	HasApp  bool   `json:"has_app"`
}

type catalogVersion struct {
	Retired bool
	Version
	Manifest Manifest
	Build    *artifact.Build
	Source   *string
}

var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$`)

func validVersion(s string) bool { return len(s) <= 64 && exactVersion.MatchString(s) }

func decodeManifest(r io.Reader) (Manifest, error) {
	var manifest Manifest
	b, e := io.ReadAll(io.LimitReader(r, MaxManifest+1))
	if e != nil || len(b) > MaxManifest {
		return manifest, registry.Bad("Package manifest exceeds its byte limit.")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	d.UseNumber()
	e = d.Decode(&manifest)
	if e != nil {
		return manifest, registry.Bad("Invalid package manifest: " + e.Error())
	}
	if e = d.Decode(new(any)); !errors.Is(e, io.EOF) {
		return manifest, registry.Bad("Expected one package manifest.")
	}
	return manifest, manifest.validate()
}

// Publish is called by the local operator command. No HTTP or agent route can
// publish bytes into the trusted catalog.
func (s *Service) Publish(ctx context.Context, r io.Reader, build *artifact.Build, files []artifact.File, source []byte) (Version, error) {
	m, e := decodeManifest(r)
	if e != nil {
		return Version{}, e
	}
	if (m.App != nil) != (build != nil && len(source) > 0) || (m.App == nil && (build != nil || len(source) > 0)) {
		return Version{}, registry.Bad("An App requires both its compiled build and source.")
	}
	directory := s.Delivery.ArtifactDirectory()
	var sourceHash any
	var buildJSON, buildHash any
	if build != nil {
		if directory == "" {
			return Version{}, registry.Conflict("Configure persistent artifact storage.")
		}
		verified, err := artifact.Inspect(files, build.Entry)
		if err != nil || verified.Hash != build.Hash {
			return Version{}, registry.Bad("Package build metadata does not match its files.")
		}
		unlock, err := s.Delivery.LockArtifacts(ctx, verified.Hash, artifact.SourceHash(source))
		if err != nil {
			return Version{}, err
		}
		defer unlock()
		if e = artifact.Store(directory, files, verified); e != nil {
			return Version{}, e
		}
		hash, err := artifact.StoreSource(directory, source)
		if err != nil {
			return Version{}, err
		}
		encoded, _ := json.Marshal(verified)
		sourceHash, buildJSON, buildHash = hash, encoded, verified.Hash
	}
	manifestJSON, _ := json.Marshal(m)
	digest := entity.Hash([]any{m, build, sourceHash})
	// A repeated digest is allowed. The duplicate update takes the same row
	// lock as the original insertion and cannot replace published content.
	_, e = s.DB.ExecContext(ctx, "INSERT INTO catalog_packages(package_key,version,digest,manifest_json,build_json,build_hash,source_hash) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE digest=digest", m.Key, m.Version, digest, manifestJSON, buildJSON, buildHash, sourceHash)
	if e != nil {
		return Version{}, e
	}
	v, e := s.lookup(ctx, s.DB, m.Key, m.Version)
	if e == nil && v.Digest != digest {
		e = registry.Conflict("This package version already identifies different content.")
	}
	return v.Version, e
}

func (s *Service) lookup(ctx context.Context, q registry.Queryer, key, version string) (catalogVersion, error) {
	var v catalogVersion
	var manifest, build []byte
	e := registry.Lookup(ctx, q, "SELECT package_key,version,digest,manifest_json,build_json,source_hash,retired_at IS NOT NULL FROM catalog_packages WHERE package_key=? AND version=?", []any{key, version}, &v.Key, &v.Version.Version, &v.Digest, &manifest, &build, &v.Source, &v.Retired)
	if e != nil {
		return v, e
	}
	v.Manifest, e = decodeManifest(strings.NewReader(string(manifest)))
	if e != nil {
		return v, fmt.Errorf("stored manifest: %w", e)
	}
	v.Name, v.HasApp = v.Manifest.Name, v.Manifest.App != nil
	v.ID = v.Key + "@" + v.Version.Version
	if build != nil {
		v.Build = new(artifact.Build)
		e = json.Unmarshal(build, v.Build)
	}
	if e == nil && entity.Hash([]any{v.Manifest, v.Build, v.Source}) != v.Digest {
		e = registry.Conflict("Stored package version failed integrity verification.")
	}
	return v, e
}

// Retire removes a version from new installation choices. Historic provenance
// and the artifacts referenced by an installed package continue to resolve.
func (s *Service) Retire(ctx context.Context, key, version string) error {
	if _, e := s.lookup(ctx, s.DB, key, version); e != nil {
		return e
	}
	_, e := s.DB.ExecContext(ctx, "UPDATE catalog_packages SET retired_at=COALESCE(retired_at,UTC_TIMESTAMP(6)) WHERE package_key=? AND version=?", key, version)
	return e
}
