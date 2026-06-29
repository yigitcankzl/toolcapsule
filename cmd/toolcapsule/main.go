package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"toolcapsule/internal/analyzer"
	"toolcapsule/internal/builder"
	"toolcapsule/internal/bundle"
	"toolcapsule/internal/cache"
	"toolcapsule/internal/dev"
	"toolcapsule/internal/doctor"
	"toolcapsule/internal/mcp"
	"toolcapsule/internal/replay"
	"toolcapsule/internal/report"
	"toolcapsule/internal/runner"
	"toolcapsule/internal/scaffold"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "toolcapsule: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		usage()
		return fmt.Errorf("missing command")
	}

	switch args[1] {
	case "init":
		return initCmd(args[2:])
	case "doctor":
		return doctorCmd(args[2:])
	case "dev":
		return devCmd(args[2:])
	case "analyze":
		return analyzeCmd(args[2:])
	case "build":
		return buildCmd(args[2:])
	case "run":
		return runCmd(args[2:])
	case "replay":
		return replayCmd(args[2:])
	case "report":
		return reportCmd(args[2:])
	case "bundle":
		return bundleCmd(args[2:])
	case "dashboard":
		return dashboardCmd(args[2:])
	case "mcp":
		return mcpCmd(args[2:])
	case "cache":
		return cacheCmd(args[2:])
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[1])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  toolcapsule init <tool-dir> [--lang go]")
	fmt.Fprintln(os.Stderr, "  toolcapsule doctor")
	fmt.Fprintln(os.Stderr, "  toolcapsule dev <tool-dir> --input <input.json>")
	fmt.Fprintln(os.Stderr, "  toolcapsule analyze <tool-dir>")
	fmt.Fprintln(os.Stderr, "  toolcapsule build <tool-dir>")
	fmt.Fprintln(os.Stderr, "  toolcapsule run <tool-dir> --input <input.json>")
	fmt.Fprintln(os.Stderr, "  toolcapsule replay <run-log.jsonl> [--run-id <id>|--latest-failed]")
	fmt.Fprintln(os.Stderr, "  toolcapsule report <run-log.jsonl> [--html] --out <report.md|report.html>")
	fmt.Fprintln(os.Stderr, "  toolcapsule bundle <run-log.jsonl> [--run-id <id>] --out <run.tcbundle>")
	fmt.Fprintln(os.Stderr, "  toolcapsule dashboard <run-log.jsonl> [--addr 127.0.0.1:8787]")
	fmt.Fprintln(os.Stderr, "  toolcapsule mcp serve|print-config|install <args>")
	fmt.Fprintln(os.Stderr, "  toolcapsule cache list|inspect <source_hash>|clean")
}

func initCmd(args []string) error {
	target, lang, err := parseInitArgs(args)
	if err != nil {
		return err
	}
	result, err := scaffold.Init(target, scaffold.Options{Lang: lang})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseInitArgs(args []string) (string, string, error) {
	var target string
	lang := "go"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--lang":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--lang requires a value")
			}
			lang = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", fmt.Errorf("unknown init flag %q", arg)
			}
			if target != "" {
				return "", "", fmt.Errorf("init accepts one <tool-dir>")
			}
			target = arg
		}
	}
	if target == "" {
		return "", "", fmt.Errorf("init requires <tool-dir>")
	}
	return target, lang, nil
}

func doctorCmd(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("doctor accepts no arguments")
	}
	return printJSON(doctor.Run())
}

func devCmd(args []string) error {
	toolDir, input, forceBuild, fallback, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	if input == "" {
		return fmt.Errorf("dev requires --input")
	}
	result, err := dev.Run(toolDir, input, dev.Options{ForceBuild: forceBuild, Fallback: fallback})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func analyzeCmd(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("analyze requires <tool-dir>")
	}

	result, err := analyzer.Analyze(fs.Arg(0))
	if err != nil {
		return err
	}
	return printJSON(result)
}

func buildCmd(args []string) error {
	toolDir, force, err := parseBuildArgs(args)
	if err != nil {
		return err
	}
	result, err := builder.Build(toolDir, builder.Options{Force: force})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseBuildArgs(args []string) (string, bool, error) {
	var toolDir string
	var force bool
	for _, arg := range args {
		switch arg {
		case "--force":
			force = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown build flag %q", arg)
			}
			if toolDir != "" {
				return "", false, fmt.Errorf("build accepts one <tool-dir>")
			}
			toolDir = arg
		}
	}
	if toolDir == "" {
		return "", false, fmt.Errorf("build requires <tool-dir>")
	}
	return toolDir, force, nil
}

func runCmd(args []string) error {
	toolDir, input, forceBuild, fallback, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	if input == "" {
		return fmt.Errorf("run requires --input")
	}

	result, err := runner.Run(toolDir, input, runner.Options{ForceBuild: forceBuild, Fallback: fallback})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseRunArgs(args []string) (string, string, bool, string, error) {
	var toolDir string
	var input string
	var forceBuild bool
	var fallback string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--input":
			if i+1 >= len(args) {
				return "", "", false, "", fmt.Errorf("--input requires a value")
			}
			input = args[i+1]
			i++
		case "--force-build":
			forceBuild = true
		case "--fallback":
			if i+1 >= len(args) {
				return "", "", false, "", fmt.Errorf("--fallback requires a value")
			}
			fallback = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, "", fmt.Errorf("unknown run flag %q", arg)
			}
			if toolDir != "" {
				return "", "", false, "", fmt.Errorf("run accepts one <tool-dir>")
			}
			toolDir = arg
		}
	}
	if toolDir == "" {
		return "", "", false, "", fmt.Errorf("run requires <tool-dir>")
	}
	return toolDir, input, forceBuild, fallback, nil
}

func cacheCmd(args []string) error {
	fs := flag.NewFlagSet("cache", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("cache supports: list, inspect <source_hash>, clean")
	}
	switch fs.Arg(0) {
	case "list":
		if fs.NArg() != 1 {
			return fmt.Errorf("cache list accepts no extra arguments")
		}
		entries, err := cache.List(".")
		if err != nil {
			return err
		}
		return printJSON(entries)
	case "inspect":
		if fs.NArg() != 2 {
			return fmt.Errorf("cache inspect requires <source_hash>")
		}
		entry, err := cache.Inspect(".", fs.Arg(1))
		if err != nil {
			return err
		}
		return printJSON(entry)
	case "clean":
		if fs.NArg() != 1 {
			return fmt.Errorf("cache clean accepts no extra arguments")
		}
		if err := cache.Clean("."); err != nil {
			return err
		}
		return printJSON(map[string]bool{"ok": true})
	default:
		return fmt.Errorf("unknown cache command %q", fs.Arg(0))
	}
}

func replayCmd(args []string) error {
	logPath, opts, err := parseReplayArgs(args)
	if err != nil {
		return err
	}
	result, err := replay.Run(logPath, opts)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseReplayArgs(args []string) (string, replay.Options, error) {
	var logPath string
	var opts replay.Options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--run-id":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--run-id requires a value")
			}
			opts.RunID = args[i+1]
			i++
		case "--latest-failed":
			opts.LatestFailed = true
		case "--backend":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--backend requires a value")
			}
			opts.Backend = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", opts, fmt.Errorf("unknown replay flag %q", arg)
			}
			if logPath != "" {
				return "", opts, fmt.Errorf("replay accepts one <run-log.jsonl>")
			}
			logPath = arg
		}
	}
	if logPath == "" {
		return "", opts, fmt.Errorf("replay requires <run-log.jsonl>")
	}
	return logPath, opts, nil
}

func reportCmd(args []string) error {
	logPath, outPath, html, err := parseReportArgs(args)
	if err != nil {
		return err
	}
	if html {
		if outPath == "" {
			htmlReport, _, err := report.GenerateHTML(logPath)
			if err != nil {
				return err
			}
			fmt.Print(htmlReport)
			return nil
		}
		result, err := report.WriteHTML(logPath, outPath)
		if err != nil {
			return err
		}
		return printJSON(result)
	}
	if outPath == "" {
		markdown, _, err := report.Generate(logPath)
		if err != nil {
			return err
		}
		fmt.Print(markdown)
		return nil
	}
	result, err := report.Write(logPath, outPath)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseReportArgs(args []string) (string, string, bool, error) {
	var logPath string
	var outPath string
	var html bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--html":
			html = true
		case "--out":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("--out requires a value")
			}
			outPath = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown report flag %q", arg)
			}
			if logPath != "" {
				return "", "", false, fmt.Errorf("report accepts one <run-log.jsonl>")
			}
			logPath = arg
		}
	}
	if logPath == "" {
		return "", "", false, fmt.Errorf("report requires <run-log.jsonl>")
	}
	return logPath, outPath, html, nil
}

func dashboardCmd(args []string) error {
	logPath, addr, err := parseDashboardArgs(args)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "dashboard listening on http://%s\n", addr)
	return report.Serve(logPath, addr)
}

func parseDashboardArgs(args []string) (string, string, error) {
	var logPath string
	addr := "127.0.0.1:8787"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--addr":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--addr requires a value")
			}
			addr = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", fmt.Errorf("unknown dashboard flag %q", arg)
			}
			if logPath != "" {
				return "", "", fmt.Errorf("dashboard accepts one <run-log.jsonl>")
			}
			logPath = arg
		}
	}
	if logPath == "" {
		return "", "", fmt.Errorf("dashboard requires <run-log.jsonl>")
	}
	return logPath, addr, nil
}

func bundleCmd(args []string) error {
	logPath, runID, out, err := parseBundleArgs(args)
	if err != nil {
		return err
	}
	result, err := bundle.Create(logPath, bundle.Options{RunID: runID, Out: out})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func parseBundleArgs(args []string) (string, string, string, error) {
	var logPath string
	var runID string
	var out string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--run-id":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--run-id requires a value")
			}
			runID = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--out requires a value")
			}
			out = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", "", fmt.Errorf("unknown bundle flag %q", arg)
			}
			if logPath != "" {
				return "", "", "", fmt.Errorf("bundle accepts one <run-log.jsonl>")
			}
			logPath = arg
		}
	}
	if logPath == "" {
		return "", "", "", fmt.Errorf("bundle requires <run-log.jsonl>")
	}
	return logPath, runID, out, nil
}

func mcpCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("mcp supports: serve <tools-root>, print-config <tools-root>, install <client> <tools-root>")
	}
	switch args[0] {
	case "serve":
		if len(args) != 2 {
			return fmt.Errorf("mcp serve requires <tools-root>")
		}
		return mcp.Serve(mcp.ServerOptions{ToolsRoot: args[1]})
	case "print-config":
		if len(args) != 2 {
			return fmt.Errorf("mcp print-config requires <tools-root>")
		}
		config, err := mcp.PrintConfig(args[1])
		if err != nil {
			return err
		}
		return printJSON(config)
	case "install":
		if len(args) != 3 {
			return fmt.Errorf("mcp install requires <client> <tools-root>")
		}
		result, err := mcp.Install(args[1], args[2])
		if err != nil {
			return err
		}
		return printJSON(result)
	default:
		return fmt.Errorf("unknown mcp command %q", args[0])
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
