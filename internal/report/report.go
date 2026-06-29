package report

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
)

type Record struct {
	RunID             string `json:"run_id"`
	ReplayOf          string `json:"replay_of,omitempty"`
	StartedAt         string `json:"started_at"`
	OK                bool   `json:"ok"`
	Tool              string `json:"tool"`
	Backend           string `json:"backend"`
	CacheHit          bool   `json:"cache_hit"`
	SourceHash        string `json:"source_hash"`
	WASMHash          string `json:"wasm_hash,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
	Input             any    `json:"input"`
	InputSchemaValid  bool   `json:"input_schema_valid"`
	Output            any    `json:"output,omitempty"`
	OutputSchemaValid bool   `json:"output_schema_valid"`
	Stdout            string `json:"stdout"`
	Stderr            string `json:"stderr"`
	ErrorType         string `json:"error_type,omitempty"`
	Error             string `json:"error,omitempty"`
}

type Result struct {
	Path    string `json:"path,omitempty"`
	Records int    `json:"records"`
	Failed  int    `json:"failed"`
}

func Generate(logPath string) (string, Result, error) {
	records, err := loadRecords(logPath)
	if err != nil {
		return "", Result{}, err
	}

	failed := 0
	for _, record := range records {
		if !record.OK {
			failed++
		}
	}

	var b bytes.Buffer
	fmt.Fprintln(&b, "# ToolCapsule Run Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Source log: `%s`\n\n", logPath)
	fmt.Fprintln(&b, "## Summary")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Total records: %d\n", len(records))
	fmt.Fprintf(&b, "- Successful: %d\n", len(records)-failed)
	fmt.Fprintf(&b, "- Failed: %d\n", failed)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Timeline")

	for _, record := range records {
		status := "failed"
		if record.OK {
			status = "ok"
		}
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "### %s\n\n", record.RunID)
		fmt.Fprintf(&b, "- Status: `%s`\n", status)
		fmt.Fprintf(&b, "- Tool: `%s`\n", record.Tool)
		fmt.Fprintf(&b, "- Backend: `%s`\n", record.Backend)
		fmt.Fprintf(&b, "- Started at: `%s`\n", record.StartedAt)
		fmt.Fprintf(&b, "- Duration: `%dms`\n", record.DurationMS)
		fmt.Fprintf(&b, "- Cache hit: `%t`\n", record.CacheHit)
		fmt.Fprintf(&b, "- Input schema valid: `%t`\n", record.InputSchemaValid)
		fmt.Fprintf(&b, "- Output schema valid: `%t`\n", record.OutputSchemaValid)
		if record.ReplayOf != "" {
			fmt.Fprintf(&b, "- Replay of: `%s`\n", record.ReplayOf)
		}
		if record.SourceHash != "" {
			fmt.Fprintf(&b, "- Source hash: `%s`\n", record.SourceHash)
		}
		if record.WASMHash != "" {
			fmt.Fprintf(&b, "- WASM hash: `%s`\n", record.WASMHash)
		}
		if record.ErrorType != "" {
			fmt.Fprintf(&b, "- Error type: `%s`\n", record.ErrorType)
			fmt.Fprintf(&b, "- Error: `%s`\n", record.Error)
		}
		writeJSONBlock(&b, "Input", record.Input)
		if record.Output != nil {
			writeJSONBlock(&b, "Output", record.Output)
		}
		if record.Stdout != "" {
			writeTextBlock(&b, "Stdout", record.Stdout)
		}
		if record.Stderr != "" {
			writeTextBlock(&b, "Stderr", record.Stderr)
		}
	}

	return b.String(), Result{Records: len(records), Failed: failed}, nil
}

func Write(logPath, outPath string) (Result, error) {
	markdown, result, err := Generate(logPath)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPath, []byte(markdown), 0o644); err != nil {
		return Result{}, err
	}
	result.Path = outPath
	return result, nil
}

func GenerateHTML(logPath string) (string, Result, error) {
	records, err := loadRecords(logPath)
	if err != nil {
		return "", Result{}, err
	}
	failed := 0
	for _, record := range records {
		if !record.OK {
			failed++
		}
	}
	data := struct {
		Log        string
		Records    []Record
		Successful int
		Failed     int
	}{Log: logPath, Records: records, Successful: len(records) - failed, Failed: failed}
	var b bytes.Buffer
	if err := htmlReportTemplate.Execute(&b, data); err != nil {
		return "", Result{}, err
	}
	return b.String(), Result{Records: len(records), Failed: failed}, nil
}

func WriteHTML(logPath, outPath string) (Result, error) {
	html, result, err := GenerateHTML(logPath)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
		return Result{}, err
	}
	result.Path = outPath
	return result, nil
}

func Serve(logPath, addr string) error {
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		html, _, err := GenerateHTML(logPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}
	http.HandleFunc("/", handler)
	return http.ListenAndServe(addr, nil)
}

func loadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no records in %s", path)
	}
	return records, nil
}

func writeJSONBlock(b *bytes.Buffer, title string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		data = []byte(fmt.Sprintf("%v", value))
	}
	fmt.Fprintf(b, "\n%s:\n\n", title)
	fmt.Fprintln(b, "```json")
	fmt.Fprintln(b, string(data))
	fmt.Fprintln(b, "```")
}

func writeTextBlock(b *bytes.Buffer, title, value string) {
	fmt.Fprintf(b, "\n%s:\n\n", title)
	fmt.Fprintln(b, "```text")
	fmt.Fprintln(b, value)
	fmt.Fprintln(b, "```")
}

var htmlReportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ToolCapsule Report</title>
  <style>
    body { font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 0; background: #0f172a; color: #e2e8f0; }
    main { max-width: 1100px; margin: 0 auto; padding: 32px; }
    .hero { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); margin: 24px 0; }
    .card { background: #111827; border: 1px solid #334155; border-radius: 16px; padding: 18px; box-shadow: 0 18px 60px rgba(0,0,0,.25); }
    .metric { font-size: 34px; font-weight: 800; }
    .muted { color: #94a3b8; }
    article { margin: 18px 0; }
    .ok { border-left: 5px solid #22c55e; }
    .failed { border-left: 5px solid #ef4444; }
    code, pre { background: #020617; border: 1px solid #1e293b; border-radius: 10px; color: #cbd5e1; }
    code { padding: 2px 6px; }
    pre { padding: 14px; overflow: auto; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 10px; }
  </style>
</head>
<body>
<main>
  <h1>ToolCapsule Run Report</h1>
  <p class="muted">Source log: <code>{{.Log}}</code></p>
  <section class="hero">
    <div class="card"><div class="metric">{{len .Records}}</div><div class="muted">records</div></div>
    <div class="card"><div class="metric">{{.Successful}}</div><div class="muted">successful</div></div>
    <div class="card"><div class="metric">{{.Failed}}</div><div class="muted">failed</div></div>
  </section>
  {{range .Records}}
  <article class="card {{if .OK}}ok{{else}}failed{{end}}">
    <h2>{{.RunID}}</h2>
    <div class="grid">
      <p>Status: <code>{{if .OK}}ok{{else}}failed{{end}}</code></p>
      <p>Tool: <code>{{.Tool}}</code></p>
      <p>Backend: <code>{{.Backend}}</code></p>
      <p>Duration: <code>{{.DurationMS}}ms</code></p>
      <p>Cache hit: <code>{{.CacheHit}}</code></p>
      <p>Input schema: <code>{{.InputSchemaValid}}</code></p>
      <p>Output schema: <code>{{.OutputSchemaValid}}</code></p>
    </div>
    {{if .ReplayOf}}<p>Replay of: <code>{{.ReplayOf}}</code></p>{{end}}
    {{if .SourceHash}}<p>Source hash: <code>{{.SourceHash}}</code></p>{{end}}
    {{if .WASMHash}}<p>WASM hash: <code>{{.WASMHash}}</code></p>{{end}}
    {{if .ErrorType}}<p>Error: <code>{{.ErrorType}}</code> {{.Error}}</p>{{end}}
    <details><summary>Input</summary><pre>{{printf "%#v" .Input}}</pre></details>
    {{if .Output}}<details><summary>Output</summary><pre>{{printf "%#v" .Output}}</pre></details>{{end}}
    {{if .Stdout}}<details><summary>Stdout</summary><pre>{{.Stdout}}</pre></details>{{end}}
    {{if .Stderr}}<details><summary>Stderr</summary><pre>{{.Stderr}}</pre></details>{{end}}
  </article>
  {{end}}
</main>
</body>
</html>`))
