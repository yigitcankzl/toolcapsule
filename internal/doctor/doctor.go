package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Command string `json:"command,omitempty"`
}

type Result struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

func Run() Result {
	checks := []Check{
		checkGo(),
		checkWasip1(),
		checkOptionalBinary("tinygo", "TOOLCAPSULE_TINYGO", "tinygo", "needed for language: tinygo"),
		checkOptionalBinary("cargo", "TOOLCAPSULE_CARGO", "cargo", "needed for language: rust"),
		checkOptionalBinary("javy", "TOOLCAPSULE_JAVY", "javy", "needed for language: javascript"),
		checkCache(),
		checkDocker(),
		checkMCPConfig("claude", filepath.Join(homeDir(), ".config", "Claude", "claude_desktop_config.json")),
		checkMCPConfig("opencode", filepath.Join(homeDir(), ".config", "opencode", "opencode.json")),
	}
	ok := true
	for _, check := range checks {
		if !check.OK && check.Name != "docker" && check.Name != "tinygo" && check.Name != "cargo" && check.Name != "javy" && !strings.HasPrefix(check.Name, "mcp_config_") {
			ok = false
		}
	}
	return Result{OK: ok, Checks: checks}
}

func GoBinary() string {
	if path := os.Getenv("TOOLCAPSULE_GO"); path != "" {
		return path
	}
	return "go"
}

func checkGo() Check {
	bin := GoBinary()
	path, err := exec.LookPath(bin)
	if err != nil {
		return Check{Name: "go", OK: false, Detail: "go binary not found", Hint: "install Go 1.22+ or set TOOLCAPSULE_GO=/path/to/go"}
	}
	out, err := runCommand(path, "version")
	if err != nil {
		return Check{Name: "go", OK: false, Detail: err.Error(), Command: path + " version"}
	}
	return Check{Name: "go", OK: true, Detail: strings.TrimSpace(out), Command: path + " version"}
}

func checkWasip1() Check {
	bin := GoBinary()
	out, err := runCommand(bin, "tool", "dist", "list")
	if err != nil {
		return Check{Name: "go_wasip1", OK: false, Detail: err.Error(), Hint: "Go 1.21+ is required for GOOS=wasip1"}
	}
	if !strings.Contains(out, "wasip1/wasm") {
		return Check{Name: "go_wasip1", OK: false, Detail: "wasip1/wasm target not listed", Hint: "upgrade Go"}
	}
	return Check{Name: "go_wasip1", OK: true, Detail: "wasip1/wasm target available"}
}

func checkCache() Check {
	dir := filepath.Join(".", ".toolcapsule", "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Check{Name: "cache_writable", OK: false, Detail: err.Error()}
	}
	tmp, err := os.CreateTemp(dir, "doctor-*")
	if err != nil {
		return Check{Name: "cache_writable", OK: false, Detail: err.Error()}
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return Check{Name: "cache_writable", OK: true, Detail: dir}
}

func checkDocker() Check {
	path, err := exec.LookPath("docker")
	if err != nil {
		return Check{Name: "docker", OK: false, Detail: "docker not found", Hint: "Docker is only needed for --fallback docker"}
	}
	out, err := runCommand(path, "--version")
	if err != nil {
		return Check{Name: "docker", OK: false, Detail: err.Error(), Hint: "Docker is only needed for --fallback docker"}
	}
	return Check{Name: "docker", OK: true, Detail: strings.TrimSpace(out), Command: path + " --version"}
}

func checkOptionalBinary(name, env, fallback, hint string) Check {
	bin := os.Getenv(env)
	if bin == "" {
		bin = fallback
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return Check{Name: name, OK: false, Detail: fallback + " not found", Hint: hint}
	}
	out, err := runCommand(path, "--version")
	if err != nil {
		return Check{Name: name, OK: false, Detail: err.Error(), Hint: hint}
	}
	return Check{Name: name, OK: true, Detail: strings.TrimSpace(out), Command: path + " --version"}
}

func checkMCPConfig(client, path string) Check {
	if path == "" || strings.HasPrefix(path, ".config") {
		return Check{Name: "mcp_config_" + client, OK: false, Detail: "home directory not available"}
	}
	if _, err := os.Stat(path); err == nil {
		return Check{Name: "mcp_config_" + client, OK: true, Detail: path}
	} else if os.IsNotExist(err) {
		return Check{Name: "mcp_config_" + client, OK: false, Detail: path, Hint: "use toolcapsule mcp print-config <tools-root>"}
	} else {
		return Check{Name: "mcp_config_" + client, OK: false, Detail: err.Error()}
	}
}

func runCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func homeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
}
