package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"toolcapsule/internal/builder"
	"toolcapsule/internal/cache"
	"toolcapsule/internal/manifest"
	"toolcapsule/internal/recorder"
	"toolcapsule/internal/runtime/docker"
	"toolcapsule/internal/runtime/wasm"
	"toolcapsule/internal/schema"
)

type Options struct {
	ForceBuild bool
	Fallback   string
}

type Result struct {
	OK                bool   `json:"ok"`
	Tool              string `json:"tool"`
	Backend           string `json:"backend"`
	CacheHit          bool   `json:"cache_hit"`
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

type record struct {
	RunID             string `json:"run_id"`
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

func Run(toolDir, inputPath string, opts Options) (Result, error) {
	m, _, err := manifest.LoadToolDir(toolDir)
	if err != nil {
		return Result{}, err
	}
	inputData, err := os.ReadFile(inputPath)
	if err != nil {
		return Result{}, err
	}
	var input any
	if err := json.Unmarshal(inputData, &input); err != nil {
		return Result{}, err
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	runID := recorder.RunID()
	sourceHash, err := toolSourceHash(toolDir, m)
	if err != nil {
		return Result{}, err
	}

	baseRecord := record{
		RunID:      runID,
		StartedAt:  startedAt,
		Tool:       m.Name,
		Backend:    "local-wasm",
		SourceHash: sourceHash,
		Input:      input,
	}

	if err := schema.ValidateFile(filepath.Join(toolDir, m.InputSchema), input); err != nil {
		res := Result{
			OK:               false,
			Tool:             m.Name,
			Backend:          "local-wasm",
			SourceHash:       sourceHash,
			InputSchemaValid: false,
			ErrorType:        "input_schema_validation_failed",
			Error:            err.Error(),
		}
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}
	baseRecord.InputSchemaValid = true
	if isPrebuiltCapsule(toolDir, m) {
		return runWASMCapsule(toolDir, m, inputData, sourceHash, baseRecord)
	}

	buildResult, err := builder.Build(toolDir, builder.Options{Force: opts.ForceBuild})
	if err != nil {
		if opts.Fallback == "docker" {
			return runDockerFallback(toolDir, m, inputData, input, sourceHash, baseRecord)
		}
		res := Result{
			OK:               false,
			Tool:             m.Name,
			Backend:          "local-wasm",
			SourceHash:       sourceHash,
			InputSchemaValid: true,
			ErrorType:        "build_failed",
			Error:            err.Error(),
		}
		baseRecord.InputSchemaValid = true
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}

	capsuleManifest, err := manifest.Load(filepath.Join(buildResult.CapsuleDir, manifest.FileName))
	if err != nil {
		return Result{}, err
	}
	baseRecord.CacheHit = buildResult.CacheHit
	return runWASMCapsule(buildResult.CapsuleDir, capsuleManifest, inputData, sourceHash, baseRecord)

}

func runWASMCapsule(capsuleDir string, capsuleManifest manifest.Manifest, inputData []byte, sourceHash string, baseRecord record) (Result, error) {
	wasmResult, runErr := wasm.Run(filepath.Join(capsuleDir, capsuleManifest.Module), inputData, wasm.Options{
		Timeout:  time.Duration(capsuleManifest.Limits.TimeoutMS) * time.Millisecond,
		MemoryMB: capsuleManifest.Limits.MemoryMB,
	})

	baseRecord.WASMHash = capsuleManifest.Capsule.WASMHash
	baseRecord.DurationMS = wasmResult.DurationMS
	baseRecord.Stdout = wasmResult.Stdout
	baseRecord.Stderr = wasmResult.Stderr

	res := Result{
		OK:               false,
		Tool:             capsuleManifest.Name,
		Backend:          "local-wasm",
		CacheHit:         baseRecord.CacheHit,
		SourceHash:       sourceHash,
		WASMHash:         capsuleManifest.Capsule.WASMHash,
		DurationMS:       wasmResult.DurationMS,
		InputSchemaValid: true,
	}
	if runErr != nil {
		res.ErrorType = "wasm_execution_failed"
		res.Error = runErr.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}

	var output any
	if err := json.Unmarshal([]byte(wasmResult.Stdout), &output); err != nil {
		res.ErrorType = "output_json_parse_failed"
		res.Error = fmt.Sprintf("stdout is not valid JSON: %v", err)
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}
	res.Output = output
	baseRecord.Output = output

	if err := schema.ValidateFile(filepath.Join(capsuleDir, capsuleManifest.OutputSchema), output); err != nil {
		res.ErrorType = "output_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}

	res.OK = true
	res.OutputSchemaValid = true
	baseRecord.OK = true
	baseRecord.OutputSchemaValid = true
	logPath, err := recorder.Append(baseRecord)
	res.ReplayLog = logPath
	if err != nil {
		return res, err
	}
	return res, nil
}

func toolSourceHash(toolDir string, m manifest.Manifest) (string, error) {
	if m.Capsule.SourceHash != "" {
		return m.Capsule.SourceHash, nil
	}
	return cache.SourceHash(toolDir)
}

func isPrebuiltCapsule(toolDir string, m manifest.Manifest) bool {
	if m.Runtime != "wasm" || m.Module == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(toolDir, m.Module)); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(toolDir, m.InputSchema)); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(toolDir, m.OutputSchema)); err != nil {
		return false
	}
	return true
}

func runDockerFallback(toolDir string, m manifest.Manifest, inputData []byte, input any, sourceHash string, baseRecord record) (Result, error) {
	baseRecord.Backend = "docker-sandbox"
	baseRecord.CacheHit = false
	res := Result{
		OK:               false,
		Tool:             m.Name,
		Backend:          "docker-sandbox",
		CacheHit:         false,
		SourceHash:       sourceHash,
		InputSchemaValid: true,
	}
	dockerResult, runErr := docker.RunGoTool(toolDir, inputData, docker.Options{
		Timeout:  time.Duration(m.Limits.TimeoutMS) * time.Millisecond,
		MemoryMB: m.Limits.MemoryMB,
		Network:  m.Permissions.Network,
	})
	res.DurationMS = dockerResult.DurationMS
	baseRecord.DurationMS = dockerResult.DurationMS
	baseRecord.Stdout = dockerResult.Stdout
	baseRecord.Stderr = dockerResult.Stderr
	if runErr != nil {
		res.ErrorType = "docker_execution_failed"
		res.Error = runErr.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}

	var output any
	if err := json.Unmarshal([]byte(dockerResult.Stdout), &output); err != nil {
		res.ErrorType = "output_json_parse_failed"
		res.Error = fmt.Sprintf("stdout is not valid JSON: %v", err)
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}
	res.Output = output
	baseRecord.Output = output
	if err := schema.ValidateFile(filepath.Join(toolDir, m.OutputSchema), output); err != nil {
		res.ErrorType = "output_schema_validation_failed"
		res.Error = err.Error()
		baseRecord.ErrorType = res.ErrorType
		baseRecord.Error = res.Error
		logPath, logErr := recorder.Append(baseRecord)
		res.ReplayLog = logPath
		if logErr != nil {
			return res, logErr
		}
		return res, nil
	}

	res.OK = true
	res.OutputSchemaValid = true
	baseRecord.OK = true
	baseRecord.OutputSchemaValid = true
	logPath, err := recorder.Append(baseRecord)
	res.ReplayLog = logPath
	if err != nil {
		return res, err
	}
	return res, nil
}
