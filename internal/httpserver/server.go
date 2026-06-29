package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"toolcapsule/internal/mcp"
	"toolcapsule/internal/recorder"
	"toolcapsule/internal/runner"
)

type Options struct {
	Addr      string
	ToolsRoot string
	Fallback  string
}

type Info struct {
	Addr      string   `json:"addr"`
	ToolsRoot string   `json:"tools_root"`
	Endpoints []string `json:"endpoints"`
}

type callRequest struct {
	Input      json.RawMessage `json:"input"`
	Arguments  json.RawMessage `json:"arguments"`
	ForceBuild bool            `json:"force_build"`
	Fallback   string          `json:"fallback"`
}

func Serve(opts Options) error {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:8080"
	}
	if opts.ToolsRoot == "" {
		opts.ToolsRoot = "."
	}
	tools, err := mcp.Discover(opts.ToolsRoot)
	if err != nil {
		return err
	}
	byName := map[string]mcp.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tools": len(tools)})
	})
	mux.HandleFunc("GET /v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, Info{Addr: opts.Addr, ToolsRoot: opts.ToolsRoot, Endpoints: endpoints()})
	})
	mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
	})
	mux.HandleFunc("POST /v1/tools/{name}/call", func(w http.ResponseWriter, r *http.Request) {
		callTool(w, r, byName, opts)
	})
	mux.HandleFunc("POST /v1/tools/{name}/run", func(w http.ResponseWriter, r *http.Request) {
		callTool(w, r, byName, opts)
	})
	mux.HandleFunc("GET /v1/runs/latest", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(recorder.LatestPath())
		if err != nil {
			writeError(w, http.StatusNotFound, "latest run not found", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})

	server := &http.Server{Addr: opts.Addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return server.ListenAndServe()
}

func callTool(w http.ResponseWriter, r *http.Request, byName map[string]mcp.Tool, opts Options) {
	name := r.PathValue("name")
	tool, ok := byName[name]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "unknown tool: " + name})
		return
	}
	defer r.Body.Close()
	var req callRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	dec.UseNumber()
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err)
		return
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err)
		return
	}
	input := req.Input
	if len(input) == 0 || string(input) == "null" {
		input = req.Arguments
	}
	if len(input) == 0 || string(input) == "null" {
		input = raw
	}
	inputPath, cleanup, err := writeTempInput(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create temp input", err)
		return
	}
	defer cleanup()

	fallback := req.Fallback
	if fallback == "" {
		fallback = opts.Fallback
	}
	result, err := runner.Run(tool.ToolDir, inputPath, runner.Options{ForceBuild: req.ForceBuild, Fallback: fallback})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tool execution failed", err)
		return
	}
	status := http.StatusOK
	if !result.OK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, result)
}

func writeTempInput(data []byte) (string, func(), error) {
	f, err := os.CreateTemp("", "toolcapsule-http-input-*.json")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string, err error) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message, "detail": err.Error()})
}

func endpoints() []string {
	return []string{
		"GET /healthz",
		"GET /readyz",
		"GET /v1/tools",
		"POST /v1/tools/{name}/call",
		"POST /v1/tools/{name}/run",
		"GET /v1/runs/latest",
	}
}

func NormalizeAddr(addr string) string {
	if strings.TrimSpace(addr) == "" {
		return "127.0.0.1:8080"
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	return addr
}

func URL(addr string) string {
	addr = NormalizeAddr(addr)
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + addr
}

func StartedMessage(addr string) string {
	return fmt.Sprintf("http server listening on %s", URL(addr))
}
