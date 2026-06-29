package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const FileName = "toolcapsule.yaml"

type Manifest struct {
	Name         string      `yaml:"name" json:"name"`
	Language     string      `yaml:"language,omitempty" json:"language,omitempty"`
	Runtime      string      `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Module       string      `yaml:"module,omitempty" json:"module,omitempty"`
	InputSchema  string      `yaml:"input_schema" json:"input_schema"`
	OutputSchema string      `yaml:"output_schema" json:"output_schema"`
	Limits       Limits      `yaml:"limits" json:"limits"`
	Permissions  Permissions `yaml:"permissions" json:"permissions"`
	Build        Build       `yaml:"build,omitempty" json:"build,omitempty"`
	Capsule      Capsule     `yaml:"capsule,omitempty" json:"capsule,omitempty"`
}

type Limits struct {
	TimeoutMS int `yaml:"timeout_ms" json:"timeout_ms"`
	MemoryMB  int `yaml:"memory_mb" json:"memory_mb"`
}

type Permissions struct {
	Network    bool   `yaml:"network" json:"network"`
	Filesystem string `yaml:"filesystem" json:"filesystem"`
}

type Build struct {
	Target string `yaml:"target" json:"target"`
}

type Capsule struct {
	SourceHash         string `yaml:"source_hash,omitempty" json:"source_hash,omitempty"`
	WASMHash           string `yaml:"wasm_hash,omitempty" json:"wasm_hash,omitempty"`
	BuiltAt            string `yaml:"built_at,omitempty" json:"built_at,omitempty"`
	Builder            string `yaml:"builder,omitempty" json:"builder,omitempty"`
	ToolCapsuleVersion string `yaml:"toolcapsule_version,omitempty" json:"toolcapsule_version,omitempty"`
}

func LoadToolDir(toolDir string) (Manifest, string, error) {
	path := filepath.Join(toolDir, FileName)
	m, err := Load(path)
	return m, path, err
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if err := Validate(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func Write(path string, m Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Validate(m Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest missing name")
	}
	if m.InputSchema == "" {
		return fmt.Errorf("manifest missing input_schema")
	}
	if m.OutputSchema == "" {
		return fmt.Errorf("manifest missing output_schema")
	}
	if m.Limits.TimeoutMS <= 0 {
		return fmt.Errorf("manifest limits.timeout_ms must be greater than 0")
	}
	if m.Limits.MemoryMB <= 0 {
		return fmt.Errorf("manifest limits.memory_mb must be greater than 0")
	}
	if m.Permissions.Filesystem == "" {
		return fmt.Errorf("manifest permissions.filesystem is required")
	}
	return nil
}
