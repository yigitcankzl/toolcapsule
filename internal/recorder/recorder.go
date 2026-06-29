package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const RunsDir = "runs"
const LatestFile = "latest.jsonl"

func Append(record any) (string, error) {
	if err := os.MkdirAll(RunsDir, 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	line := append(data, '\n')

	path := TodayPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(LatestPath(), line, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func RunID() string {
	return "run_" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func TodayPath() string {
	return filepath.Join(RunsDir, time.Now().UTC().Format("20060102")+".jsonl")
}

func LatestPath() string {
	return filepath.Join(RunsDir, LatestFile)
}
