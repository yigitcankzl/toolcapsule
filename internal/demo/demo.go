package demo

import (
	"path/filepath"

	"toolcapsule/internal/bundle"
	"toolcapsule/internal/recorder"
	"toolcapsule/internal/report"
	"toolcapsule/internal/runner"
)

type Options struct {
	ReportOut string
	BundleOut string
}

type Step struct {
	Name            string        `json:"name"`
	ExpectedFailure bool          `json:"expected_failure,omitempty"`
	OK              bool          `json:"ok"`
	Result          runner.Result `json:"result"`
}

type Result struct {
	OK     bool     `json:"ok"`
	Log    string   `json:"log"`
	Report string   `json:"report,omitempty"`
	Bundle string   `json:"bundle,omitempty"`
	Next   []string `json:"next"`
	Steps  []Step   `json:"steps"`
}

func Run(opts Options) (Result, error) {
	if opts.ReportOut == "" {
		opts.ReportOut = filepath.Join(recorder.RunsDir, "demo-report.html")
	}
	if opts.BundleOut == "" {
		opts.BundleOut = filepath.Join(recorder.RunsDir, "demo.tcbundle")
	}

	steps := []Step{}
	run := func(name, tool, input string, expectedFailure bool) error {
		res, err := runner.Run(tool, input, runner.Options{})
		if err != nil {
			return err
		}
		stepOK := res.OK
		if expectedFailure {
			stepOK = !res.OK
		}
		steps = append(steps, Step{Name: name, ExpectedFailure: expectedFailure, OK: stepOK, Result: res})
		return nil
	}

	if err := run("redact pii", "examples/tools/redact_pii", "examples/inputs/pii.json", false); err != nil {
		return Result{}, err
	}
	if err := run("parse csv", "examples/tools/parse_csv", "examples/inputs/csv.json", false); err != nil {
		return Result{}, err
	}
	if err := run("validate json", "examples/tools/validate_json", "examples/inputs/validate_json.json", false); err != nil {
		return Result{}, err
	}
	if err := run("query readonly table", "examples/tools/query_table_readonly", "examples/inputs/query_table.json", false); err != nil {
		return Result{}, err
	}
	if err := run("reject invalid input", "examples/tools/redact_pii", "examples/inputs/invalid_schema.json", true); err != nil {
		return Result{}, err
	}
	if err := run("detect bad output", "examples/tools/bad_output_schema_demo", "examples/inputs/empty.json", true); err != nil {
		return Result{}, err
	}
	if err := run("final bundleable run", "examples/tools/parse_csv", "examples/inputs/csv.json", false); err != nil {
		return Result{}, err
	}

	reportResult, err := report.WriteHTML(recorder.TodayPath(), opts.ReportOut)
	if err != nil {
		return Result{}, err
	}
	bundleResult, err := bundle.Create(recorder.LatestPath(), bundle.Options{Out: opts.BundleOut})
	if err != nil {
		return Result{}, err
	}

	ok := true
	for _, step := range steps {
		if !step.OK {
			ok = false
		}
	}
	return Result{
		OK:     ok,
		Log:    recorder.TodayPath(),
		Report: reportResult.Path,
		Bundle: bundleResult.Path,
		Next: []string{
			"toolcapsule replay",
			"toolcapsule report --html --out report.html",
			"toolcapsule replay " + bundleResult.Path,
			"toolcapsule mcp serve ./examples/tools",
		},
		Steps: steps,
	}, nil
}
