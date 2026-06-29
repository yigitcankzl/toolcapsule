package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"toolcapsule/internal/manifest"
)

type Result struct {
	Capsulable bool     `json:"capsulable"`
	Language   string   `json:"language"`
	Target     string   `json:"target"`
	Reasons    []string `json:"reasons"`
	Fallback   string   `json:"fallback,omitempty"`
}

var blockedGoImports = []string{
	"os/exec",
	"net/http",
	"syscall",
}

func Analyze(toolDir string) (Result, error) {
	m, _, err := manifest.LoadToolDir(toolDir)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Capsulable: true,
		Language:   m.Language,
		Target:     m.Build.Target,
	}
	if result.Target == "" {
		result.Target = "wasip1"
	}

	if m.Language != "go" {
		result.Capsulable = false
		result.Reasons = append(result.Reasons, fmt.Sprintf("unsupported language %q", m.Language))
		result.Fallback = "sandbox"
		return result, nil
	}

	if _, err := os.Stat(filepath.Join(toolDir, "go.mod")); err != nil {
		result.Capsulable = false
		result.Reasons = append(result.Reasons, "missing go.mod")
	}

	hasMain, risky, err := scanGoFiles(toolDir)
	if err != nil {
		return Result{}, err
	}
	if !hasMain {
		result.Capsulable = false
		result.Reasons = append(result.Reasons, "missing package main")
	}
	for _, item := range risky {
		result.Capsulable = false
		result.Reasons = append(result.Reasons, "blocked import "+item)
	}

	if !result.Capsulable {
		result.Fallback = "sandbox"
	}
	return result, nil
}

func scanGoFiles(root string) (bool, []string, error) {
	hasMain := false
	risky := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".toolcapsule" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		if strings.Contains(content, "package main") {
			hasMain = true
		}
		for _, imp := range blockedGoImports {
			if strings.Contains(content, "\""+imp+"\"") || strings.Contains(content, "`"+imp+"`") {
				risky[imp] = true
			}
		}
		return nil
	})
	if err != nil {
		return false, nil, err
	}

	items := make([]string, 0, len(risky))
	for item := range risky {
		items = append(items, item)
	}
	return hasMain, items, nil
}
