package schema

import (
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Schema = jsonschema.Schema

func Load(path string) (*Schema, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	return compiler.Compile(fileURL(abs))
}

func ValidateFile(schemaPath string, value any) error {
	s, err := Load(schemaPath)
	if err != nil {
		return err
	}
	return Validate(s, value)
}

func Validate(s *Schema, value any) error {
	return s.Validate(value)
}

func fileURL(path string) string {
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
