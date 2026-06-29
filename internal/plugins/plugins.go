package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"toolcapsule/internal/bundle"
	"toolcapsule/internal/manifest"
	"toolcapsule/internal/runner"
)

const metadataFile = "plugin.json"

var pluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type InstallOptions struct {
	Name          string
	PublicKey     string
	AllowUnsigned bool
}

type RunOptions struct {
	Input string
}

type Metadata struct {
	Name         string `json:"name"`
	Tool         string `json:"tool"`
	Path         string `json:"path"`
	SourceBundle string `json:"source_bundle,omitempty"`
	SourceHash   string `json:"source_hash,omitempty"`
	WASMHash     string `json:"wasm_hash,omitempty"`
	InstalledAt  string `json:"installed_at"`
	Signed       bool   `json:"signed"`
	Verified     bool   `json:"verified"`
	PublicKey    string `json:"public_key,omitempty"`
	KeyID        string `json:"key_id,omitempty"`
}

type InstallResult struct {
	OK       bool     `json:"ok"`
	Plugin   Metadata `json:"plugin"`
	Warnings []string `json:"warnings,omitempty"`
}

type ListResult struct {
	Plugins []Metadata `json:"plugins"`
}

func Home() (string, error) {
	if home := os.Getenv("TOOLCAPSULE_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".toolcapsule"), nil
}

func Dir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "plugins"), nil
}

func Resolve(name string) (string, bool) {
	if !validName(name) {
		return "", false
	}
	root, err := Dir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(path, manifest.FileName)); err != nil {
		return "", false
	}
	return path, true
}

func Install(path string, opts InstallOptions) (InstallResult, error) {
	extracted, cleanup, err := bundle.Extract(path)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	verify, err := bundle.VerifyDir(extracted.Dir, bundle.VerifyOptions{PublicKey: opts.PublicKey})
	if err != nil {
		return InstallResult{}, err
	}
	if !verify.Signed && !opts.AllowUnsigned {
		return InstallResult{}, fmt.Errorf("plugin bundle is unsigned; pass --allow-unsigned to install anyway")
	}

	m, err := manifest.Load(filepath.Join(extracted.Dir, manifest.FileName))
	if err != nil {
		return InstallResult{}, err
	}
	name := opts.Name
	if name == "" {
		name = m.Name
	}
	if !validName(name) {
		return InstallResult{}, fmt.Errorf("invalid plugin name %q", name)
	}
	root, err := Dir()
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return InstallResult{}, err
	}
	finalDir := filepath.Join(root, name)
	tmpDir, err := os.MkdirTemp(root, ".tmp-"+name+"-")
	if err != nil {
		return InstallResult{}, err
	}
	moved := false
	defer func() {
		if !moved {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	if err := copyDir(tmpDir, extracted.Dir); err != nil {
		return InstallResult{}, err
	}
	metadata := Metadata{
		Name:         name,
		Tool:         m.Name,
		Path:         finalDir,
		SourceBundle: path,
		SourceHash:   m.Capsule.SourceHash,
		WASMHash:     m.Capsule.WASMHash,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
		Signed:       verify.Signed,
		Verified:     verify.Verified,
		PublicKey:    verify.PublicKey,
		KeyID:        verify.KeyID,
	}
	if err := writeMetadata(filepath.Join(tmpDir, metadataFile), metadata); err != nil {
		return InstallResult{}, err
	}
	if err := os.RemoveAll(finalDir); err != nil {
		return InstallResult{}, err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		return InstallResult{}, err
	}
	moved = true
	metadata.Path = finalDir
	return InstallResult{OK: true, Plugin: metadata, Warnings: verify.Warnings}, nil
}

func List() (ListResult, error) {
	root, err := Dir()
	if err != nil {
		return ListResult{}, err
	}
	items, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return ListResult{Plugins: []Metadata{}}, nil
	}
	if err != nil {
		return ListResult{}, err
	}
	plugins := []Metadata{}
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		if metadata, err := Inspect(item.Name()); err == nil {
			plugins = append(plugins, metadata)
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return ListResult{Plugins: plugins}, nil
}

func Inspect(name string) (Metadata, error) {
	path, ok := Resolve(name)
	if !ok {
		return Metadata{}, fmt.Errorf("plugin %q not found", name)
	}
	data, err := os.ReadFile(filepath.Join(path, metadataFile))
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	metadata.Path = path
	return metadata, nil
}

func Remove(name string) (map[string]any, error) {
	path, ok := Resolve(name)
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	if err := os.RemoveAll(path); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "removed": name}, nil
}

func Run(name string, opts RunOptions) (runner.Result, error) {
	path, ok := Resolve(name)
	if !ok {
		return runner.Result{}, fmt.Errorf("plugin %q not found", name)
	}
	return runner.Run(path, opts.Input, runner.Options{})
}

func validName(name string) bool {
	return name != "" && pluginNamePattern.MatchString(name)
}

func copyDir(dst, src string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		outPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		return copyFile(outPath, path)
	})
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeMetadata(path string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
