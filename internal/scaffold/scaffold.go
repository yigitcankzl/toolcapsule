package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Options struct {
	Lang string
}

type Result struct {
	Name string `json:"name"`
	Lang string `json:"lang"`
	Path string `json:"path"`
}

func Init(target string, opts Options) (Result, error) {
	if opts.Lang == "" {
		opts.Lang = "go"
	}
	if opts.Lang != "go" {
		return Result{}, fmt.Errorf("unsupported init language %q", opts.Lang)
	}
	if target == "" {
		return Result{}, fmt.Errorf("init requires a target name or path")
	}

	path := filepath.Clean(target)
	name := sanitizeName(filepath.Base(path))
	if name == "" {
		return Result{}, fmt.Errorf("invalid tool name %q", target)
	}
	if err := os.MkdirAll(filepath.Join(path, "examples"), 0o755); err != nil {
		return Result{}, err
	}

	files := map[string]string{
		"go.mod":                                fmt.Sprintf("module %s\n\ngo 1.22\n", name),
		"main.go":                               mainTemplate,
		"toolcapsule.yaml":                      fmt.Sprintf(manifestTemplate, name),
		"input.schema.json":                     inputSchemaTemplate,
		"output.schema.json":                    outputSchemaTemplate,
		filepath.Join("examples", "input.json"): exampleInputTemplate,
	}
	for rel, content := range files {
		filePath := filepath.Join(path, rel)
		if _, err := os.Stat(filePath); err == nil {
			return Result{}, fmt.Errorf("refusing to overwrite existing file %s", filePath)
		} else if !os.IsNotExist(err) {
			return Result{}, err
		}
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			return Result{}, err
		}
	}
	return Result{Name: name, Lang: opts.Lang, Path: path}, nil
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = regexp.MustCompile(`[^a-z0-9_\-]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_-")
	if name == "" {
		return ""
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "tool_" + name
	}
	return strings.ReplaceAll(name, "-", "_")
}

const mainTemplate = `package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type input struct {
	Text string ` + "`json:\"text\"`" + `
}

type output struct {
	Text   string ` + "`json:\"text\"`" + `
	Length int    ` + "`json:\"length\"`" + `
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}

	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		fail(err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(output{Text: in.Text, Length: len([]rune(in.Text))}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
`

const manifestTemplate = `name: %s
language: go
input_schema: input.schema.json
output_schema: output.schema.json
limits:
  timeout_ms: 3000
  memory_mb: 32
permissions:
  network: false
  filesystem: none
build:
  target: wasip1
`

const inputSchemaTemplate = `{
  "type": "object",
  "required": ["text"],
  "additionalProperties": false,
  "properties": {
    "text": { "type": "string" }
  }
}
`

const outputSchemaTemplate = `{
  "type": "object",
  "required": ["text", "length"],
  "additionalProperties": false,
  "properties": {
    "text": { "type": "string" },
    "length": { "type": "integer" }
  }
}
`

const exampleInputTemplate = `{
  "text": "hello from ToolCapsule"
}
`
