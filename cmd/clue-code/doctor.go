package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/clue-code/clue-code/internal/adapters/aider"
	"github.com/clue-code/clue-code/internal/config"
	"github.com/clue-code/clue-code/internal/orchestrator"
	"github.com/clue-code/clue-code/internal/version"
)

// runDoctor inspects the local environment and prints a health report.
func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	fmt.Println("CLUE CODE — doctor")
	fmt.Println("====================")
	fmt.Printf("Build:       %s\n", version.String())
	fmt.Printf("OS / arch:   %s / %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Go runtime:  %s\n", runtime.Version())
	fmt.Printf("Logical CPU: %d\n", runtime.NumCPU())

	fmt.Println()
	fmt.Println("Configuration:")
	cfg := config.Load()
	fmt.Printf("  Mode:        %s\n", cfg.Mode)
	if *verbose {
		for _, t := range []config.Tier{config.TierL0, config.TierL1, config.TierL2, config.TierL3} {
			fmt.Printf("  Model %s:    %s\n", t, cfg.ModelByTier[t])
		}
	}
	if path, err := config.ConfigPath(); err == nil {
		fmt.Printf("  Config path: %s\n", path)
	}

	fmt.Println()
	fmt.Println("System resources:")
	checkRAM()
	checkDisk()

	fmt.Println()
	fmt.Println("External dependencies:")
	checkAider()
	checkOllama()
	checkNetwork()
	checkMLX()
	checkBinary("python3", "required for LoRA pipeline (Phase 5+)")
	checkBinary("git", "required for repo-aware operations")

	fmt.Println()
	fmt.Println("Agents:")
	dir := agentsDir()
	reg := orchestrator.NewRegistry()
	if errs := reg.LoadFromDir(dir); len(errs) > 0 {
		fmt.Printf("  Could not fully load agents from %s:\n", dir)
		for _, e := range errs {
			fmt.Printf("    - %v\n", e)
		}
	}
	if reg.Count() == 0 {
		fmt.Printf("  No agents found in %s\n", dir)
	} else {
		fmt.Printf("  %d agent(s) loaded from %s\n", reg.Count(), dir)
		for _, name := range reg.Names() {
			fmt.Printf("    - %s\n", name)
		}
	}

	fmt.Println()
	fmt.Println("Status: ready.")
}

// checkRAM reads total physical RAM and prints it.
// Uses sysctl on Darwin, /proc/meminfo on Linux.
func checkRAM() {
	mb, err := readTotalRAMMB()
	if err != nil {
		fmt.Printf("  ⚠ %-22s could not determine (%v)\n", "RAM", err)
		return
	}
	gb := float64(mb) / 1024.0
	if mb < 8*1024 {
		fmt.Printf("  ⚠ %-22s %.1f GB (recommended: 16 GB+)\n", "RAM", gb)
	} else {
		fmt.Printf("  ✓ %-22s %.1f GB\n", "RAM", gb)
	}
}

// readTotalRAMMB returns total physical RAM in megabytes.
func readTotalRAMMB() (uint64, error) {
	switch runtime.GOOS {
	case "darwin":
		return readRAMDarwin()
	case "linux":
		return readRAMLinux()
	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func readRAMDarwin() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, err
	}
	bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, err
	}
	return bytes / (1024 * 1024), nil
}

func readRAMLinux() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("unexpected MemTotal format")
			}
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, err
			}
			return kb / 1024, nil
		}
	}
	return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
}

// checkDisk checks free disk space on the project root partition.
func checkDisk() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	freeMB, err := freeDiskMB(cwd)
	if err != nil {
		fmt.Printf("  ⚠ %-22s could not determine (%v)\n", "disk free", err)
		return
	}
	gb := float64(freeMB) / 1024.0
	if freeMB < 500 {
		fmt.Printf("  ✗ %-22s %.1f GB free — WARNING: <500 MB (builds may fail)\n", "disk free", gb)
	} else if freeMB < 2*1024 {
		fmt.Printf("  ⚠ %-22s %.1f GB free (recommended: 2 GB+)\n", "disk free", gb)
	} else {
		fmt.Printf("  ✓ %-22s %.1f GB free\n", "disk free", gb)
	}
}

// freeDiskMB returns free disk space in MB for the filesystem containing path.
func freeDiskMB(path string) (uint64, error) {
	out, err := exec.Command("df", "-k", path).Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output")
	}
	// df -k output: Filesystem 1K-blocks Used Available Capacity Mounted
	// Field index varies between Linux (col 3) and Darwin (col 3).
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df fields: %v", fields)
	}
	kb, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse available: %w", err)
	}
	return kb / 1024, nil
}

// checkAider probes for the aider binary using the adapter's own detection
// logic so that the doctor output matches what the adapter would actually use.
func checkAider() {
	c := aider.NewClient()
	if c.Available() {
		fmt.Printf("  ✓ %-22s %s\n", "aider", c.Version())
	} else {
		fmt.Printf("  ✗ %-22s not found (optional, fallback edit available)\n", "aider")
	}
}

// checkOllama queries the local Ollama API for its version (timeout 2s).
func checkOllama() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := ollamaVersion(ctx)
	if err != nil {
		fmt.Printf("  ⚠ %-22s not running (optional — start with `ollama serve`)\n", "ollama")
		return
	}
	fmt.Printf("  ✓ %-22s %s\n", "ollama", out)
}

// ollamaVersion fetches the Ollama version string from the local API.
// Exported as a variable so tests can replace it.
var ollamaVersion = func(ctx context.Context) (string, error) {
	return runCurlJSON(ctx, "http://localhost:11434/api/version", "version")
}

// checkNetwork tests TCP connectivity to api.deepseek.com:443 with a 5s timeout.
func checkNetwork() {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.Dial("tcp", "api.deepseek.com:443")
	if err != nil {
		fmt.Printf("  ✗ %-22s cannot reach api.deepseek.com:443 (%v)\n", "network (deepseek)", err)
		return
	}
	conn.Close()
	fmt.Printf("  ✓ %-22s api.deepseek.com:443 reachable\n", "network (deepseek)")
}

// checkMLX probes for the MLX inference server binary or Python package.
// This is best-effort: it never returns a fatal error.
func checkMLX() {
	// Prefer the standalone mlx_lm.server binary if present.
	if _, err := os.Stat("/usr/local/bin/mlx_lm.server"); err == nil {
		fmt.Printf("  ✓ %-22s binary at /usr/local/bin/mlx_lm.server\n", "mlx (inference)")
		return
	}
	// Fall back to checking whether the Python package is importable.
	if _, err := exec.LookPath("python3"); err == nil {
		out, err2 := exec.Command("python3", "-c", "import mlx_lm; print(mlx_lm.__version__)").Output()
		if err2 == nil {
			fmt.Printf("  ✓ %-22s python package v%s\n", "mlx (inference)", strings.TrimSpace(string(out)))
			return
		}
	}
	fmt.Printf("  ⚠ %-22s not found (optional, Apple Silicon only)\n", "mlx (inference)")
}

// checkBinary checks for a generic binary in PATH.
func checkBinary(bin, purpose string) {
	if path, err := exec.LookPath(bin); err == nil {
		fmt.Printf("  ✓ %-22s found at %s\n", bin, path)
	} else {
		fmt.Printf("  ✗ %-22s NOT found  — %s\n", bin, purpose)
	}
}

// runCurlJSON invokes curl to fetch a JSON endpoint and extracts a string field.
// This avoids importing a JSON library for a simple version probe.
func runCurlJSON(ctx context.Context, url, field string) (string, error) {
	out, err := exec.CommandContext(ctx, "curl", "-s", "--max-time", "2", url).Output()
	if err != nil {
		return "", err
	}
	// Simple extraction: find `"field":"value"` or `"field": "value"`.
	body := string(out)
	key := `"` + field + `"`
	idx := strings.Index(body, key)
	if idx < 0 {
		return "", fmt.Errorf("field %q not found in response", field)
	}
	rest := body[idx+len(key):]
	rest = strings.TrimLeft(rest, ` :`)
	if len(rest) == 0 || rest[0] != '"' {
		return "", fmt.Errorf("unexpected value format")
	}
	end := strings.Index(rest[1:], `"`)
	if end < 0 {
		return "", fmt.Errorf("unterminated string value")
	}
	return rest[1 : end+1], nil
}

// agentsDir resolves the directory holding agent definitions.
// Resolution order:
//  1. CLUE_CODE_AGENTS_DIR environment variable (absolute path).
//  2. ./agents relative to the current working directory.
//  3. ./agents relative to the binary location (fallback).
func agentsDir() string {
	if v := os.Getenv("CLUE_CODE_AGENTS_DIR"); v != "" {
		return v
	}
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "agents")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "agents"
	}
	return filepath.Join(filepath.Dir(exe), "agents")
}
