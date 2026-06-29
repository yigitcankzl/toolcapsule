package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	wasmPath, err := buildWASM(toolDir, tmpDir, m)
	if err != nil {
		return Result{}, err
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

func buildWASM(toolDir, tmpDir string, m manifest.Manifest) (string, error) {
	language := normalizeLanguage(m.Language)
	switch language {
	case "go":
		return buildGo(toolDir, tmpDir)
	case "tinygo":
		return buildTinyGo(toolDir, tmpDir, m.Build.Target)
	case "rust":
		return buildRust(toolDir, m.Build.Target)
	case "javascript":
		return buildJavaScript(toolDir, tmpDir)
	default:
		return "", fmt.Errorf("unsupported build language %q", m.Language)
	}
}

func buildGo(toolDir, tmpDir string) (string, error) {
	wasmPath := filepath.Join(tmpDir, "tool.wasm")
	cmd := exec.Command(goBinary(), "build", "-o", wasmPath, ".")
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go wasi build failed: %w: %s", err, string(output))
	}
	return wasmPath, nil
}

func buildTinyGo(toolDir, tmpDir, target string) (string, error) {
	if target == "" {
		target = "wasi"
	}
	wasmPath := filepath.Join(tmpDir, "tool.wasm")
	cmd := exec.Command(toolBinary("TOOLCAPSULE_TINYGO", "tinygo"), "build", "-target", target, "-o", wasmPath, ".")
	cmd.Dir = toolDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tinygo wasi build failed: %w: %s", err, string(output))
	}
	return wasmPath, nil
}

func buildRust(toolDir, target string) (string, error) {
	if target == "" {
		target = "wasm32-wasip1"
	}
	cmd := exec.Command(toolBinary("TOOLCAPSULE_CARGO", "cargo"), "build", "--target", target, "--release")
	cmd.Dir = toolDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cargo wasi build failed: %w: %s", err, string(output))
	}
	matches, err := filepath.Glob(filepath.Join(toolDir, "target", target, "release", "*.wasm"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("cargo build produced no wasm artifact")
	}
	return matches[0], nil
}

func buildJavaScript(toolDir, tmpDir string) (string, error) {
	entry := filepath.Join(toolDir, "index.js")
	if _, err := os.Stat(entry); err != nil {
		entry = filepath.Join(toolDir, "main.js")
	}
	wasmPath := filepath.Join(tmpDir, "tool.wasm")
	cmd := exec.Command(toolBinary("TOOLCAPSULE_JAVY", "javy"), "build", entry, "-o", wasmPath)
	cmd.Dir = toolDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("javy build failed: %w: %s", err, string(output))
	}
	return wasmPath, nil
}

func goBinary() string {
	if path := os.Getenv("TOOLCAPSULE_GO"); path != "" {
		return path
	}
	return "go"
}

func toolBinary(env, fallback string) string {
	if path := os.Getenv(env); path != "" {
		return path
	}
	return fallback
}

func normalizeLanguage(language string) string {
	switch strings.ToLower(language) {
	case "js", "javascript", "javy":
		return "javascript"
	case "tinygo":
		return "tinygo"
	case "rust", "rs":
		return "rust"
	default:
		return strings.ToLower(language)
	}
}
