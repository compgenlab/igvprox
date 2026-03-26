package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Genome            string
	BrowserURL        string
	SocketPath        string
	AllowMissingIndex bool
}

func Load(explicitPath string) (Config, error) {
	path, err := resolveConfigPath(explicitPath)
	if err != nil {
		return Config{}, err
	}
	if path == "" {
		return Config{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	cfg := Config{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("parse config %q:%d: expected key = value", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		switch key {
		case "genome":
			cfg.Genome = value
		case "browser_url":
			cfg.BrowserURL = value
		case "socket_path":
			cfg.SocketPath = value
		case "allow_missing_index":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return Config{}, fmt.Errorf("parse config %q:%d: %w", path, lineNo, err)
			}
			cfg.AllowMissingIndex = parsed
		default:
			return Config{}, fmt.Errorf("parse config %q:%d: unknown key %q", path, lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	return cfg, nil
}

func ResolveSocketPath(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return expandUser(configured)
	}

	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		return filepath.Join(runtimeDir, "igvprox.sock"), nil
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("igvprox-%d.sock", os.Getuid())), nil
}

func resolveConfigPath(explicitPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return expandUser(explicitPath)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	candidates := []string{
		filepath.Join(home, ".config", "igvprox", "config.toml"),
		filepath.Join(home, ".igvproxrc"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat config %q: %w", candidate, err)
		}
	}
	return "", nil
}

func expandUser(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	if path[1] != '/' {
		return "", fmt.Errorf("unsupported home expansion in path %q", path)
	}
	return filepath.Join(home, path[2:]), nil
}
