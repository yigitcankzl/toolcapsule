package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

type Options struct {
	Timeout time.Duration
}

type Result struct {
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"duration_ms"`
}

func RunGoTool(toolDir string, input []byte, opts Options) (Result, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	absToolDir, err := filepath.Abs(toolDir)
	if err != nil {
		return Result{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	started := time.Now()
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-i", "-v", absToolDir+":/work", "-w", "/work", "golang:1.22", "go", "run", ".")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	duration := time.Since(started)
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration, DurationMS: duration.Milliseconds()}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("docker execution timed out after %s", opts.Timeout)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
