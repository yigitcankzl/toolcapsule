package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"toolcapsule/internal/manifest"
)

const Version = "0.1.0"

type Entry struct {
	SourceHash string `json:"source_hash"`
	Path       string `json:"path"`
	Tool       string `json:"tool,omitempty"`
	WASMHash   string `json:"wasm_hash,omitempty"`
	BuiltAt    string `json:"built_at,omitempty"`
	Manifest   string `json:"manifest,omitempty"`
}

type BuildInfo struct {
	Tool               string `json:"tool"`
	SourceHash         string `json:"source_hash"`
	WASMHash           string `json:"wasm_hash"`
	Builder            string `json:"builder"`
	ToolCapsuleVersion string `json:"toolcapsule_version"`
	BuiltAt            string `json:"built_at"`
}

func Root(projectRoot string) string {
	return filepath.Join(projectRoot, ".toolcapsule", "cache")
}

func CapsuleDir(projectRoot, sourceHash string) string {
	return filepath.Join(Root(projectRoot), "capsules", sourceHash)
}

func Exists(projectRoot, sourceHash string) bool {
	dir := CapsuleDir(projectRoot, sourceHash)
	for _, name := range []string{"tool.wasm", manifest.FileName, "build.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	if err != nil {
		return false
	}
	for _, name := range []string{m.InputSchema, m.OutputSchema} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func SourceHash(toolDir string) (string, error) {
	files, err := hashableFiles(toolDir)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	_, _ = io.WriteString(h, "toolcapsule-source-v1\n")
	_, _ = io.WriteString(h, "toolcapsule-version:"+Version+"\n")
	for _, path := range files {
		rel, err := filepath.Rel(toolDir, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, rel+"\n")
		h.Write(data)
		_, _ = io.WriteString(h, "\n")
	}
	return "sha256_" + hex.EncodeToString(h.Sum(nil)), nil
}

func FileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256_" + hex.EncodeToString(sum[:]), nil
}

func Save(projectRoot, toolDir, sourceHash, wasmPath string, m manifest.Manifest) (Entry, error) {
	finalDir := CapsuleDir(projectRoot, sourceHash)
	capsulesRoot := filepath.Dir(finalDir)
	if err := os.MkdirAll(capsulesRoot, 0o755); err != nil {
		return Entry{}, err
	}
	tmpDir, err := os.MkdirTemp(capsulesRoot, ".tmp-"+sourceHash+"-")
	if err != nil {
		return Entry{}, err
	}
	moved := false
	defer func() {
		if !moved {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	destWASM := filepath.Join(tmpDir, "tool.wasm")
	if err := copyFile(destWASM, wasmPath); err != nil {
		return Entry{}, err
	}
	wasmHash, err := FileHash(destWASM)
	if err != nil {
		return Entry{}, err
	}

	builtAt := time.Now().UTC().Format(time.RFC3339)
	sourceInputSchema := m.InputSchema
	sourceOutputSchema := m.OutputSchema
	m.Runtime = "wasm"
	m.Module = "tool.wasm"
	m.InputSchema = filepath.Base(m.InputSchema)
	m.OutputSchema = filepath.Base(m.OutputSchema)
	m.Capsule.SourceHash = sourceHash
	m.Capsule.WASMHash = wasmHash
	m.Capsule.BuiltAt = builtAt
	m.Capsule.Builder = "go-wasip1-local"
	m.Capsule.ToolCapsuleVersion = Version

	if err := copyFile(filepath.Join(tmpDir, m.InputSchema), filepath.Join(toolDir, sourceInputSchema)); err != nil {
		return Entry{}, fmt.Errorf("copy input schema: %w", err)
	}
	if err := copyFile(filepath.Join(tmpDir, m.OutputSchema), filepath.Join(toolDir, sourceOutputSchema)); err != nil {
		return Entry{}, fmt.Errorf("copy output schema: %w", err)
	}
	if err := manifest.Write(filepath.Join(tmpDir, manifest.FileName), m); err != nil {
		return Entry{}, err
	}

	info := BuildInfo{
		Tool:               m.Name,
		SourceHash:         sourceHash,
		WASMHash:           wasmHash,
		Builder:            m.Capsule.Builder,
		ToolCapsuleVersion: Version,
		BuiltAt:            builtAt,
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "build.json"), data, 0o644); err != nil {
		return Entry{}, err
	}
	if err := os.RemoveAll(finalDir); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		return Entry{}, err
	}
	moved = true

	return Entry{SourceHash: sourceHash, Path: finalDir, Tool: m.Name, WASMHash: wasmHash, BuiltAt: builtAt}, nil
}

func List(projectRoot string) ([]Entry, error) {
	base := filepath.Join(Root(projectRoot), "capsules")
	items, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		if strings.HasPrefix(item.Name(), ".tmp-") {
			continue
		}
		dir := filepath.Join(base, item.Name())
		entry := Entry{SourceHash: item.Name(), Path: dir}
		infoData, err := os.ReadFile(filepath.Join(dir, "build.json"))
		if err == nil {
			var info BuildInfo
			if json.Unmarshal(infoData, &info) == nil {
				entry.Tool = info.Tool
				entry.WASMHash = info.WASMHash
				entry.BuiltAt = info.BuiltAt
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SourceHash < entries[j].SourceHash })
	return entries, nil
}

func Inspect(projectRoot, sourceHash string) (Entry, error) {
	dir := CapsuleDir(projectRoot, sourceHash)
	if !Exists(projectRoot, sourceHash) {
		return Entry{}, fmt.Errorf("cache entry %s not found", sourceHash)
	}
	entry := Entry{SourceHash: sourceHash, Path: dir, Manifest: filepath.Join(dir, manifest.FileName)}
	infoData, err := os.ReadFile(filepath.Join(dir, "build.json"))
	if err == nil {
		var info BuildInfo
		if json.Unmarshal(infoData, &info) == nil {
			entry.Tool = info.Tool
			entry.WASMHash = info.WASMHash
			entry.BuiltAt = info.BuiltAt
		}
	}
	return entry, nil
}

func Clean(projectRoot string) error {
	return os.RemoveAll(Root(projectRoot))
}

func hashableFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == ".toolcapsule" || name == "dist" || name == "runs" || name == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".wasm") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
