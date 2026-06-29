package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"toolcapsule/internal/analyzer"
	"toolcapsule/internal/cache"
	"toolcapsule/internal/manifest"
)

type Options struct {
	Force bool
}

type Result struct {
	Tool       string          `json:"tool"`
	CacheHit   bool            `json:"cache_hit"`
	SourceHash string          `json:"source_hash"`
	CapsuleDir string          `json:"capsule_dir"`
	Analysis   analyzer.Result `json:"analysis"`
}

func Build(toolDir string, opts Options) (Result, error) {
	m, _, err := manifest.LoadToolDir(toolDir)
	if err != nil {
		return Result{}, err
	}
	analysis, err := analyzer.Analyze(toolDir)
	if err != nil {
		return Result{}, err
	}
	if !analysis.Capsulable {
		return Result{Tool: m.Name, Analysis: analysis}, fmt.Errorf("tool is not capsulable: %v", analysis.Reasons)
	}

	sourceHash, err := cache.SourceHash(toolDir)
	if err != nil {
		return Result{}, err
	}
	projectRoot := "."
	if cache.Exists(projectRoot, sourceHash) && !opts.Force {
		return Result{
			Tool:       m.Name,
			CacheHit:   true,
			SourceHash: sourceHash,
			CapsuleDir: cache.CapsuleDir(projectRoot, sourceHash),
			Analysis:   analysis,
		}, nil
	}

	tmpDir, err := os.MkdirTemp("", "toolcapsule-build-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmpDir)

	wasmPath := filepath.Join(tmpDir, "tool.wasm")
	cmd := exec.Command(goBinary(), "build", "-o", wasmPath, ".")
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("go wasi build failed: %w: %s", err, string(output))
	}

	entry, err := cache.Save(projectRoot, toolDir, sourceHash, wasmPath, m)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Tool:       m.Name,
		CacheHit:   false,
		SourceHash: sourceHash,
		CapsuleDir: entry.Path,
		Analysis:   analysis,
	}, nil
}

func goBinary() string {
	if path := os.Getenv("TOOLCAPSULE_GO"); path != "" {
		return path
	}
	return "go"
}
