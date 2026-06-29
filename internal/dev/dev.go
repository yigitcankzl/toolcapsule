package dev

import (
	"toolcapsule/internal/analyzer"
	"toolcapsule/internal/builder"
	"toolcapsule/internal/runner"
)

type Options struct {
	ForceBuild bool
	Fallback   string
}

type Result struct {
	Analysis analyzer.Result `json:"analysis"`
	Build    any             `json:"build,omitempty"`
	Run      runner.Result   `json:"run"`
}

func Run(toolDir, inputPath string, opts Options) (Result, error) {
	analysis, err := analyzer.Analyze(toolDir)
	if err != nil {
		return Result{}, err
	}
	var build any
	if analysis.Capsulable {
		buildResult, err := builder.Build(toolDir, builder.Options{Force: opts.ForceBuild})
		if err != nil {
			return Result{}, err
		}
		build = buildResult
	}
	runResult, err := runner.Run(toolDir, inputPath, runner.Options{ForceBuild: opts.ForceBuild, Fallback: opts.Fallback})
	if err != nil {
		return Result{}, err
	}
	return Result{Analysis: analysis, Build: build, Run: runResult}, nil
}
