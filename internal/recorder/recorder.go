package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func Append(record any) (string, error) {
	if err := os.MkdirAll("runs", 0o755); err != nil {
		return "", err
	}
	path := filepath.Join("runs", time.Now().UTC().Format("20060102")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

func RunID() string {
	return "run_" + time.Now().UTC().Format("20060102T150405.000000000Z")
}
