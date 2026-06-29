package wasm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type Options struct {
	Timeout  time.Duration
	MemoryMB int
}

type Result struct {
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"duration_ms"`
}

func Run(wasmPath string, input []byte, opts Options) (Result, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	moduleBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return Result{}, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	started := time.Now()
	runtimeConfig := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	if opts.MemoryMB > 0 {
		runtimeConfig = runtimeConfig.WithMemoryLimitPages(memoryLimitPages(opts.MemoryMB))
	}
	// Persist wazero's compiled module next to the capsule so cold starts skip
	// recompiling the (large) Go wasm module on every run.
	// This stays per-capsule; switch to a shared dir if disk dedup matters.
	if cache, cacheErr := wazero.NewCompilationCacheWithDir(filepath.Join(filepath.Dir(wasmPath), "wazero")); cacheErr == nil {
		runtimeConfig = runtimeConfig.WithCompilationCache(cache)
		defer cache.Close(ctx)
	}
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	defer runtime.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	config := wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(input)).
		WithStdout(&stdout).
		WithStderr(&stderr)

	_, err = runtime.InstantiateWithConfig(ctx, moduleBytes, config)
	duration := time.Since(started)
	result := Result{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Duration:   duration,
		DurationMS: duration.Milliseconds(),
	}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("wasm execution timed out after %s", opts.Timeout)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func memoryLimitPages(memoryMB int) uint32 {
	bytes := uint64(memoryMB) * 1024 * 1024
	pages := bytes / 65536
	if bytes%65536 != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	if pages > 65536 {
		pages = 65536
	}
	return uint32(pages)
}
