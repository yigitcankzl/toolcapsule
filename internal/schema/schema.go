package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Schema struct {
	Type                 string            `json:"type"`
	Required             []string          `json:"required"`
	Properties           map[string]Schema `json:"properties"`
	Items                *Schema           `json:"items"`
	AdditionalProperties bool              `json:"additionalProperties"`
}

func Load(path string) (Schema, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Schema{}, err
	}
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return Schema{}, err
	}
	return s, nil
}

func ValidateFile(schemaPath string, value any) error {
	s, err := Load(schemaPath)
	if err != nil {
		return err
	}
	return Validate(s, value)
}

func Validate(s Schema, value any) error {
	return validateAt("$", s, value)
}

func validateAt(path string, s Schema, value any) error {
	if s.Type == "" {
		return nil
	}

	switch s.Type {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be object", path)
		}
		for _, key := range s.Required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("%s missing required field %q", path, key)
			}
		}
		for key, child := range s.Properties {
			if v, ok := obj[key]; ok {
				if err := validateAt(path+"."+key, child, v); err != nil {
					return err
				}
			}
		}
		if !s.AdditionalProperties {
			for key := range obj {
				if _, ok := s.Properties[key]; !ok {
					return fmt.Errorf("%s unknown field %q", path, key)
				}
			}
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be array", path)
		}
		if s.Items != nil {
			for i, item := range arr {
				if err := validateAt(fmt.Sprintf("%s[%d]", path, i), *s.Items, item); err != nil {
					return err
				}
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be string", path)
		}
	case "number":
		switch value.(type) {
		case float64, int, int64, uint64:
			return nil
		default:
			return fmt.Errorf("%s must be number", path)
		}
	case "integer":
		f, ok := value.(float64)
		if !ok || f != float64(int64(f)) {
			return fmt.Errorf("%s must be integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
	default:
		return fmt.Errorf("%s unsupported schema type %q", path, s.Type)
	}
	return nil
}
