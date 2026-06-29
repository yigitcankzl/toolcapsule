package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type input struct {
	Text string `json:"text"`
}

type output struct {
	Text       string   `json:"text"`
	Redactions []string `json:"redactions"`
}

var patterns = []struct {
	Name string
	Re   *regexp.Regexp
}{
	{Name: "email", Re: regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)},
	{Name: "phone", Re: regexp.MustCompile(`(?:\+?\d[\d .\-()]{7,}\d)`)},
	{Name: "api_key", Re: regexp.MustCompile(`(?i)(api[_-]?key|token|secret)[:=][A-Za-z0-9_\-]{12,}`)},
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

	text := in.Text
	seen := map[string]bool{}
	var redactions []string
	for _, pattern := range patterns {
		if pattern.Re.MatchString(text) {
			seen[pattern.Name] = true
			text = pattern.Re.ReplaceAllString(text, "["+strings.ToUpper(pattern.Name)+"]")
		}
	}
	for _, pattern := range patterns {
		if seen[pattern.Name] {
			redactions = append(redactions, pattern.Name)
		}
	}

	if err := json.NewEncoder(os.Stdout).Encode(output{Text: text, Redactions: redactions}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
