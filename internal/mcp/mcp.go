package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"toolcapsule/internal/manifest"
	"toolcapsule/internal/runner"
)

type ServerOptions struct {
	ToolsRoot string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

type Tool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	ToolDir      string          `json:"-"`
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func Serve(opts ServerOptions) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.ToolsRoot == "" {
		opts.ToolsRoot = "."
	}
	tools, err := Discover(opts.ToolsRoot)
	if err != nil {
		return err
	}
	byName := map[string]Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	enc := json.NewEncoder(opts.Stdout)
	scanner := bufio.NewScanner(opts.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(errorResponse(nil, -32700, err.Error()))
			continue
		}
		if req.ID == nil && (strings.HasPrefix(req.Method, "notifications/") || req.Method == "initialized") {
			continue
		}
		resp := handle(req, tools, byName)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func Discover(root string) ([]Tool, error) {
	var tools []Tool
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".toolcapsule" || name == "runs" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != manifest.FileName {
			return nil
		}
		toolDir := filepath.Dir(path)
		m, err := manifest.Load(path)
		if err != nil {
			return err
		}
		inputSchema, err := os.ReadFile(filepath.Join(toolDir, m.InputSchema))
		if err != nil {
			return err
		}
		outputSchema, err := os.ReadFile(filepath.Join(toolDir, m.OutputSchema))
		if err != nil {
			return err
		}
		tools = append(tools, Tool{
			Name:         m.Name,
			Title:        strings.ReplaceAll(m.Name, "_", " "),
			Description:  "ToolCapsule managed tool " + m.Name,
			InputSchema:  json.RawMessage(inputSchema),
			OutputSchema: json.RawMessage(outputSchema),
			ToolDir:      toolDir,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

func PrintConfig(toolsRoot string) (map[string]any, error) {
	exe := os.Getenv("TOOLCAPSULE_BIN")
	if exe == "" {
		path, err := os.Executable()
		if err != nil {
			return nil, err
		}
		exe = path
	}
	absRoot, err := filepath.Abs(toolsRoot)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mcpServers": map[string]any{
			"toolcapsule": map[string]any{
				"command": exe,
				"args":    []string{"mcp", "serve", absRoot},
			},
		},
	}, nil
}

func Install(client, toolsRoot string) (map[string]any, error) {
	path, err := configPath(client)
	if err != nil {
		return nil, err
	}
	toolcapsuleConfig, err := PrintConfig(toolsRoot)
	if err != nil {
		return nil, err
	}
	config := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("parse existing MCP config %s: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		config["mcpServers"] = servers
	}
	newServers, _ := toolcapsuleConfig["mcpServers"].(map[string]any)
	servers["toolcapsule"] = newServers["toolcapsule"]
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "client": client, "path": path}, nil
}

func configPath(client string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch client {
	case "claude":
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "opencode.json"), nil
	default:
		return "", fmt.Errorf("unsupported MCP client %q", client)
	}
}

func handle(req request, tools []Tool, byName map[string]Tool) response {
	switch req.Method {
	case "initialize":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "toolcapsule", "version": "0.1.0"},
		}}
	case "ping":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		publicTools := make([]Tool, 0, len(tools))
		for _, tool := range tools {
			publicTools = append(publicTools, tool)
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": publicTools}}
	case "tools/call":
		return callTool(req, byName)
	default:
		return errorResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

func callTool(req request, byName map[string]Tool) response {
	var params callParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, err.Error())
	}
	tool, ok := byName[params.Name]
	if !ok {
		return errorResponse(req.ID, -32602, "unknown tool: "+params.Name)
	}
	inputFile, err := os.CreateTemp("", "toolcapsule-mcp-input-*.json")
	if err != nil {
		return errorResponse(req.ID, -32603, err.Error())
	}
	inputPath := inputFile.Name()
	defer os.Remove(inputPath)
	if err := json.NewEncoder(inputFile).Encode(params.Arguments); err != nil {
		_ = inputFile.Close()
		return errorResponse(req.ID, -32603, err.Error())
	}
	if err := inputFile.Close(); err != nil {
		return errorResponse(req.ID, -32603, err.Error())
	}
	result, err := runner.Run(tool.ToolDir, inputPath, runner.Options{})
	if err != nil {
		return errorResponse(req.ID, -32603, err.Error())
	}
	text, _ := json.Marshal(result)
	toolResult := map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(text)}},
		"isError": !result.OK,
	}
	if result.OK {
		toolResult["structuredContent"] = result.Output
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult}
}

func errorResponse(id any, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: map[string]any{"code": code, "message": message}}
}
