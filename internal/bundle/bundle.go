package bundle

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"toolcapsule/internal/cache"
	"toolcapsule/internal/manifest"
)

type Options struct {
	RunID string
	Out   string
}

type Record struct {
	RunID      string `json:"run_id"`
	OK         bool   `json:"ok"`
	Tool       string `json:"tool"`
	Backend    string `json:"backend"`
	SourceHash string `json:"source_hash"`
	WASMHash   string `json:"wasm_hash,omitempty"`
	Input      any    `json:"input"`
}

type Result struct {
	Path       string `json:"path"`
	RunID      string `json:"run_id"`
	Tool       string `json:"tool"`
	SourceHash string `json:"source_hash"`
}

type Extracted struct {
	Dir    string
	Record Record
}

func Create(logPath string, opts Options) (Result, error) {
	if opts.Out == "" {
		return Result{}, fmt.Errorf("bundle requires --out")
	}
	records, err := loadRecords(logPath)
	if err != nil {
		return Result{}, err
	}
	record, err := selectRecord(records, opts.RunID)
	if err != nil {
		return Result{}, err
	}
	if record.SourceHash == "" {
		return Result{}, fmt.Errorf("selected record has no source_hash")
	}
	if !cache.Exists(".", record.SourceHash) {
		return Result{}, fmt.Errorf("capsule %s not found in cache", record.SourceHash)
	}
	capsuleDir := cache.CapsuleDir(".", record.SourceHash)
	m, err := manifest.Load(filepath.Join(capsuleDir, manifest.FileName))
	if err != nil {
		return Result{}, err
	}

	out, err := os.Create(opts.Out)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	recordData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := addBytes(tw, "record.json", recordData); err != nil {
		return Result{}, err
	}
	for _, name := range []string{manifest.FileName, m.Module, m.InputSchema, m.OutputSchema, "build.json"} {
		if err := addFile(tw, capsuleDir, name); err != nil {
			return Result{}, err
		}
	}
	return Result{Path: opts.Out, RunID: record.RunID, Tool: record.Tool, SourceHash: record.SourceHash}, nil
}

func Extract(path string) (Extracted, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return Extracted{}, nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Extracted{}, nil, err
	}
	defer gz.Close()
	dir, err := os.MkdirTemp("", "toolcapsule-bundle-*")
	if err != nil {
		return Extracted{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return Extracted{}, nil, err
		}
		name := filepath.Clean(h.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			cleanup()
			return Extracted{}, nil, fmt.Errorf("unsafe bundle path %q", h.Name)
		}
		outPath := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			cleanup()
			return Extracted{}, nil, err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode))
		if err != nil {
			cleanup()
			return Extracted{}, nil, err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			cleanup()
			return Extracted{}, nil, copyErr
		}
		if closeErr != nil {
			cleanup()
			return Extracted{}, nil, closeErr
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		cleanup()
		return Extracted{}, nil, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		cleanup()
		return Extracted{}, nil, err
	}
	return Extracted{Dir: dir, Record: record}, cleanup, nil
}

func loadRecords(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []Record
	for lineNo, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, lineNo+1, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no records in %s", path)
	}
	return records, nil
}

func selectRecord(records []Record, runID string) (Record, error) {
	if runID == "" {
		return records[len(records)-1], nil
	}
	for _, record := range records {
		if record.RunID == runID {
			return record, nil
		}
	}
	return Record{}, fmt.Errorf("run_id %q not found", runID)
}

func addFile(tw *tar.Writer, root, name string) error {
	path := filepath.Join(root, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return addBytes(tw, name, data)
}

func addBytes(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
