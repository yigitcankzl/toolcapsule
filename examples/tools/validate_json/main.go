package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type input struct {
	Document any               `json:"document"`
	Required []string          `json:"required"`
	Types    map[string]string `json:"types"`
}

type output struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
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
	obj, ok := in.Document.(map[string]any)
	if !ok {
		write(output{Valid: false, Errors: []string{"document must be object"}})
		return
	}
	errors := []string{}
	for _, key := range in.Required {
		if _, ok := obj[key]; !ok {
			errors = append(errors, "missing required field: "+key)
		}
	}
	for key, typ := range in.Types {
		if value, ok := obj[key]; ok && !matchesType(value, typ) {
			errors = append(errors, key+" must be "+typ)
		}
	}
	write(output{Valid: len(errors) == 0, Errors: errors})
}

func matchesType(value any, typ string) bool {
	switch typ {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return true
	}
}

func write(out output) {
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
