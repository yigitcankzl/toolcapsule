package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"toolcapsule/internal/bundle"
	"toolcapsule/internal/cache"
	"toolcapsule/internal/manifest"
	"toolcapsule/internal/recorder"
	"toolcapsule/internal/runtime/wasm"
	"toolcapsule/internal/schema"
)

type Options struct {
	RunID        string
	LatestFailed bool
	Backend      string
}

type SourceRecord struct {
	RunID      string `json:"run_id"`
	OK         bool   `json:"ok"`
	Tool       string `json:"tool"`
	Backend    string `json:"backend"`
	SourceHash string `json:"source_hash"`
	WASMHash   string `json:"wasm_hash,omitempty"`
	Input      any    `json:"input"`
}

type Result struct {
	OK                bool   `json:"ok"`
	ReplayOf          string `json:"replay_of"`
	Tool              string `json:"tool"`
	Backend           string `json:"backend"`
	SourceHash        string `json:"source_hash"`
	WASMHash          string `json:"wasm_hash,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
	InputSchemaValid  bool   `json:"input_schema_valid"`
	OutputSchemaValid bool   `json:"output_schema_valid"`
	Output            any    `json:"output,omitempty"`
	ErrorType         string `json:"error_type,omitempty"`
	Error             string `json:"error,omitempty"`
	ReplayLog         string `json:"replay_log,omitempty"`
}

type replayRecord struct {
	RunID             string `json:"run_id"`
	ReplayOf          string `json:"replay_of"`
	StartedAt         string `json:"started_at"`
	OK                bool   `json:"ok"`
	Tool              string `json:"tool"`
	Backend           string `json:"backend"`
	CacheHit          bool   `json:"cache_hit"`
	SourceHash        string `json:"source_hash"`
	WASMHash          string `json:"wasm_hash,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
	Input             any    `json:"input"`
	InputSchemaValid  bool   `json:"input_schema_valid"`
	Output            any    `json:"output,omitempty"`
	OutputSchemaValid bool   `json:"output_schema_valid"`
	Stdout            string `json:"stdout"`
	Stderr            string `json:"stderr"`
	ErrorType         string `json:"error_type,omitempty"`
	Error             string `json:"error,omitempty"`
}

func Run(logPath string, opts Options) (Result, error) {
	if isBundle(logPath) {
		return runBundle(logPath)
	}
	if opts.Backend == "" {
		opts.Backend = "local"
	}
	if opts.Backend != "local" {
		return Result{}, fmt.Errorf("unsupported replay backend %q", opts.Backend)
	}

	records, err := LoadRecords(logPath)
	if err != nil {
		return Result{}, err
	}
	source, err := selectRecord(records, opts)
	if err != nil {
		return Result{}, err
	}
	if source.SourceHash == "" {
		return Result{}, fmt.Errorf("selected record has no source_hash")
	}
	if source.Input == nil {
		return Result{}, fmt.Errorf("selected record has no input")
	}
	if !cache.Exists(".", source.SourceHash) {
		return Result{}, fmt.Errorf("capsule %s not found in cache", source.SourceHash)
	}

	capsuleDir := cache.CapsuleDir(".", source.SourceHash)
	capsuleManifest, err := manifest.Load(filepath.Join(capsuleDir, manifest.FileName))
	if err != nil {
		return Result{}, err
	}
	inputData, err := json.Marshal(source.Input)
	if err != nil {
		return Result{}, err
	}

	baseRecord := replayRecord{
		RunID:      recorder.RunID(),
		ReplayOf:   source.RunID,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Tool:       capsuleManifest.Name,
		Backend:    "local-wasm-replay",
		CacheHit:   true,
		SourceHash: source.SourceHash,
		WASMHash:   capsuleManifest.Capsule.WASMHash,
		Input:      source.Input,
	}
	res := Result{
		OK:         false,
		ReplayOf:   source.RunID,
		Tool:       capsuleManifest.Name,
		Backend:    "local-wasm-replay",
		SourceHash: source.SourceHash,
		WASMHash:   capsuleManifest.Capsule.WASMHash,
	}

	if err := schema.ValidateFile(filepath.Join(capsuleDir, capsuleManifest.InputSchema), source.Input); err != nil {
		res.ErrorType = "input_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	res.InputSchemaValid = true
	baseRecord.InputSchemaValid = true

	wasmResult, runErr := wasm.Run(filepath.Join(capsuleDir, capsuleManifest.Module), inputData, wasm.Options{
		Timeout:  time.Duration(capsuleManifest.Limits.TimeoutMS) * time.Millisecond,
		MemoryMB: capsuleManifest.Limits.MemoryMB,
	})
	res.DurationMS = wasmResult.DurationMS
	baseRecord.DurationMS = wasmResult.DurationMS
	baseRecord.Stdout = wasmResult.Stdout
	baseRecord.Stderr = wasmResult.Stderr
	if runErr != nil {
		res.ErrorType = "wasm_execution_failed"
		res.Error = runErr.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}

	var output any
	if err := json.Unmarshal([]byte(wasmResult.Stdout), &output); err != nil {
		res.ErrorType = "output_json_parse_failed"
		res.Error = fmt.Sprintf("stdout is not valid JSON: %v", err)
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	res.Output = output
	baseRecord.Output = output

	if err := schema.ValidateFile(filepath.Join(capsuleDir, capsuleManifest.OutputSchema), output); err != nil {
		res.ErrorType = "output_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}

	res.OK = true
	res.OutputSchemaValid = true
	baseRecord.OK = true
	baseRecord.OutputSchemaValid = true
	return finish(res, baseRecord)
}

func runBundle(path string) (Result, error) {
	extracted, cleanup, err := bundle.Extract(path)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	capsuleManifest, err := manifest.Load(filepath.Join(extracted.Dir, manifest.FileName))
	if err != nil {
		return Result{}, err
	}
	inputData, err := json.Marshal(extracted.Record.Input)
	if err != nil {
		return Result{}, err
	}
	baseRecord := replayRecord{
		RunID:      recorder.RunID(),
		ReplayOf:   extracted.Record.RunID,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Tool:       capsuleManifest.Name,
		Backend:    "bundle-wasm-replay",
		CacheHit:   false,
		SourceHash: extracted.Record.SourceHash,
		WASMHash:   capsuleManifest.Capsule.WASMHash,
		Input:      extracted.Record.Input,
	}
	res := Result{OK: false, ReplayOf: extracted.Record.RunID, Tool: capsuleManifest.Name, Backend: "bundle-wasm-replay", SourceHash: extracted.Record.SourceHash, WASMHash: capsuleManifest.Capsule.WASMHash}
	if err := schema.ValidateFile(filepath.Join(extracted.Dir, capsuleManifest.InputSchema), extracted.Record.Input); err != nil {
		res.ErrorType = "input_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	res.InputSchemaValid = true
	baseRecord.InputSchemaValid = true
	wasmResult, runErr := wasm.Run(filepath.Join(extracted.Dir, capsuleManifest.Module), inputData, wasm.Options{Timeout: time.Duration(capsuleManifest.Limits.TimeoutMS) * time.Millisecond, MemoryMB: capsuleManifest.Limits.MemoryMB})
	res.DurationMS = wasmResult.DurationMS
	baseRecord.DurationMS = wasmResult.DurationMS
	baseRecord.Stdout = wasmResult.Stdout
	baseRecord.Stderr = wasmResult.Stderr
	if runErr != nil {
		res.ErrorType = "wasm_execution_failed"
		res.Error = runErr.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	var output any
	if err := json.Unmarshal([]byte(wasmResult.Stdout), &output); err != nil {
		res.ErrorType = "output_json_parse_failed"
		res.Error = fmt.Sprintf("stdout is not valid JSON: %v", err)
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	res.Output = output
	baseRecord.Output = output
	if err := schema.ValidateFile(filepath.Join(extracted.Dir, capsuleManifest.OutputSchema), output); err != nil {
		res.ErrorType = "output_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		return finish(res, baseRecord)
	}
	res.OK = true
	res.OutputSchemaValid = true
	baseRecord.OK = true
	baseRecord.OutputSchemaValid = true
	return finish(res, baseRecord)
}

func isBundle(path string) bool {
	return filepath.Ext(path) == ".tcbundle"
}

func LoadRecords(path string) ([]SourceRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []SourceRecord
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" {
			continue
		}
		var record SourceRecord
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no records in %s", path)
	}
	return records, nil
}

func selectRecord(records []SourceRecord, opts Options) (SourceRecord, error) {
	if opts.RunID != "" && opts.LatestFailed {
		return SourceRecord{}, fmt.Errorf("--run-id and --latest-failed cannot be used together")
	}
	if opts.RunID != "" {
		for _, record := range records {
			if record.RunID == opts.RunID {
				return record, nil
			}
		}
		return SourceRecord{}, fmt.Errorf("run_id %q not found", opts.RunID)
	}
	if opts.LatestFailed {
		for i := len(records) - 1; i >= 0; i-- {
			if !records[i].OK {
				return records[i], nil
			}
		}
		return SourceRecord{}, fmt.Errorf("no failed records found")
	}
	return records[len(records)-1], nil
}

func finish(res Result, record replayRecord) (Result, error) {
	path, err := recorder.Append(record)
	res.ReplayLog = path
	if err != nil {
		return res, err
	}
	return res, nil
}
