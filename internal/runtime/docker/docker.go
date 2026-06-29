package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type Options struct {
	Timeout  time.Duration
	MemoryMB int
	Network  bool
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
	if opts.Timeout < 60*time.Second {
		opts.Timeout = 60 * time.Second
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
	image := os.Getenv("TOOLCAPSULE_DOCKER_GO_IMAGE")
	if image == "" {
		image = "golang:1.22"
	}
	args := []string{
		"run", "--rm", "-i",
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--cpus", "1",
		"--read-only",
		"--tmpfs", "/tmp:rw,exec,nosuid,size=512m,mode=1777",
		"-e", "GOCACHE=/tmp/gocache",
		"-e", "GOMODCACHE=/tmp/gomodcache",
		"-v", absToolDir + ":/work:ro",
		"-w", "/work",
	}
	if opts.MemoryMB > 0 {
		memoryMB := opts.MemoryMB
		if memoryMB < 1024 {
			memoryMB = 1024
		}
		args = append(args, "--memory", strconv.Itoa(memoryMB)+"m")
	}
	if !opts.Network {
		args = append(args, "--network=none")
	}
	args = append(args, image, "go", "run", ".")
	cmd := exec.CommandContext(ctx, "docker", args...)
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
