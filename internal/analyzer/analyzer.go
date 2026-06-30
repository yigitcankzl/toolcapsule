package analyzer

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"toolcapsule/internal/manifest"
)

type Result struct {
	Capsulable bool     `json:"capsulable"`
	Language   string   `json:"language"`
	Target     string   `json:"target"`
	Reasons    []string `json:"reasons"`
	Imports    []string `json:"imports,omitempty"`
	Fallback   string   `json:"fallback,omitempty"`
}

var blockedGoImports = map[string]string{
	"C":        "uses cgo",
	"os/exec":  "can spawn processes",
	"net":      "network sockets are not available in local WASM",
	"net/http": "HTTP is not available in local WASM",
	"plugin":   "plugins require native dynamic loading",
	"syscall":  "direct syscalls are not available in WASM",
	"unsafe":   "unsafe code is blocked by policy",
}

func Analyze(toolDir string) (Result, error) {
	m, _, err := manifest.LoadToolDir(toolDir)
	if err != nil {
		return Result{}, err
	}

	language := normalizeLanguage(m.Language)
	result := Result{Capsulable: true, Language: language, Target: defaultTarget(language, m.Build.Target)}

	switch language {
	case "go":
		analyzeGo(toolDir, &result)
	case "tinygo":
		analyzeTinyGo(toolDir, &result)
	case "rust":
		analyzeRust(toolDir, &result)
	case "javascript":
		analyzeJavaScript(toolDir, &result)
	default:
		result.Capsulable = false
		result.Reasons = append(result.Reasons, fmt.Sprintf("unsupported language %q", m.Language))
	}

	if !result.Capsulable {
		result.Fallback = "sandbox"
	}
	sort.Strings(result.Reasons)
	sort.Strings(result.Imports)
	return result, nil
}

func analyzeGo(toolDir string, result *Result) {
	if _, err := os.Stat(filepath.Join(toolDir, "go.mod")); err != nil {
		block(result, "missing go.mod")
	}
	hasMain, imports, reasons, err := scanGoFiles(toolDir)
	if err != nil {
		block(result, err.Error())
		return
	}
	result.Imports = imports
	if !hasMain {
		block(result, "missing package main")
	}
	for _, reason := range reasons {
		block(result, reason)
	}
	for _, reason := range goListReasons(toolDir) {
		block(result, reason)
	}
}

func analyzeTinyGo(toolDir string, result *Result) {
	if _, err := lookPathEnv("TOOLCAPSULE_TINYGO", "tinygo"); err != nil {
		block(result, "tinygo binary not found")
	}
	hasMain, imports, reasons, err := scanGoFiles(toolDir)
	if err != nil {
		block(result, err.Error())
		return
	}
	result.Imports = imports
	if !hasMain {
		block(result, "missing package main")
	}
	for _, reason := range reasons {
		block(result, reason)
	}
}

func analyzeRust(toolDir string, result *Result) {
	if _, err := os.Stat(filepath.Join(toolDir, "Cargo.toml")); err != nil {
		block(result, "missing Cargo.toml")
	}
	if _, err := lookPathEnv("TOOLCAPSULE_CARGO", "cargo"); err != nil {
		block(result, "cargo binary not found")
	}
	for _, reason := range scanTextForRisk(toolDir, ".rs", map[string]string{
		"std::process": "uses std::process",
		"std::net":     "uses std::net",
		"unsafe {":     "uses unsafe Rust block",
	}) {
		block(result, reason)
	}
}

func analyzeJavaScript(toolDir string, result *Result) {
	if _, err := lookPathEnv("TOOLCAPSULE_JAVY", "javy"); err != nil {
		block(result, "javy binary not found")
	}
	if !fileExists(filepath.Join(toolDir, "index.js")) && !fileExists(filepath.Join(toolDir, "main.js")) {
		block(result, "missing index.js or main.js")
	}
	for _, reason := range scanTextForRisk(toolDir, ".js", map[string]string{
		"child_process":    "uses child_process",
		"require('net')":   "uses net module",
		"require(\"net\")": "uses net module",
		"fetch(":           "uses network fetch",
	}) {
		block(result, reason)
	}
}

func scanGoFiles(root string) (bool, []string, []string, error) {
	hasMain := false
	imports := map[string]bool{}
	reasons := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".toolcapsule" || name == "dist" || name == "runs" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		if file.Name != nil && file.Name.Name == "main" {
			hasMain = true
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, "`\"")
			imports[path] = true
			if reason, blocked := blockedGoImports[path]; blocked {
				reasons["blocked import "+path+": "+reason] = true
			}
		}
		return nil
	})
	if err != nil {
		return false, nil, nil, err
	}
	return hasMain, keys(imports), keys(reasons), nil
}

func goListReasons(toolDir string) []string {
	goBin := os.Getenv("TOOLCAPSULE_GO")
	if goBin == "" {
		goBin = "go"
	}
	cmd := exec.Command(goBin, "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		} else {
			detail = err.Error() + ": " + detail
		}
		return []string{"go list failed: " + detail}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		imp := strings.TrimSpace(line)
		if imp == "" || seen[imp] {
			continue
		}
		seen[imp] = true
		if imp == "unsafe" {
			continue
		}
		if reason, blocked := blockedGoImports[imp]; blocked {
			return []string{"blocked dependency " + imp + ": " + reason}
		}
	}
	return nil
}

func scanTextForRisk(root, suffix string, blocked map[string]string) []string {
	reasons := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		for needle, reason := range blocked {
			if strings.Contains(content, needle) {
				reasons[reason] = true
			}
		}
		return nil
	})
	return keys(reasons)
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

func defaultTarget(language, target string) string {
	if target != "" {
		return target
	}
	switch language {
	case "rust":
		return "wasm32-wasip1"
	case "tinygo":
		return "wasi"
	default:
		return "wasip1"
	}
}

func lookPathEnv(env, fallback string) (string, error) {
	if path := os.Getenv(env); path != "" {
		return path, nil
	}
	return exec.LookPath(fallback)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func block(result *Result, reason string) {
	if reason == "" {
		return
	}
	result.Capsulable = false
	result.Reasons = append(result.Reasons, reason)
}

func keys(m map[string]bool) []string {
	items := make([]string, 0, len(m))
	for item := range m {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}
