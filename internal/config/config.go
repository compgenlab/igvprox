package config

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
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
	ConstantTracks    []Track
	Path              string
}

type Track struct {
	Name     string
	URL      string
	IndexURL string
	Format   string
	Type     string
	Genome   string
	Enabled  bool
}

func Load(explicitPath string) (Config, error) {
	path, err := resolveConfigPath(explicitPath)
	if err != nil {
		return Config{}, err
	}
	if path == "" {
		path, err = defaultConfigPath()
		if err != nil {
			return Config{}, err
		}
		cfg := defaultConfig()
		cfg.Path = path
		if err := writeDefaultConfig(path, cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	cfg := defaultConfig()
	cfg.ConstantTracks = nil
	cfg.Path = path
	scanner := bufio.NewScanner(file)
	lineNo := 0
	currentTrack := -1
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
		if line == "[[constant_track]]" {
			cfg.ConstantTracks = append(cfg.ConstantTracks, Track{})
			currentTrack = len(cfg.ConstantTracks) - 1
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("parse config %q:%d: expected key = value", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		if currentTrack >= 0 {
			switch key {
			case "name":
				cfg.ConstantTracks[currentTrack].Name = value
			case "url":
				cfg.ConstantTracks[currentTrack].URL = value
			case "index_url":
				cfg.ConstantTracks[currentTrack].IndexURL = value
			case "format":
				cfg.ConstantTracks[currentTrack].Format = value
			case "type":
				cfg.ConstantTracks[currentTrack].Type = value
			case "genome":
				cfg.ConstantTracks[currentTrack].Genome = value
			case "enabled":
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return Config{}, fmt.Errorf("parse config %q:%d: %w", path, lineNo, err)
				}
				cfg.ConstantTracks[currentTrack].Enabled = parsed
			default:
				return Config{}, fmt.Errorf("parse config %q:%d: unknown constant_track key %q", path, lineNo, key)
			}
			continue
		}
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

	candidates, err := defaultConfigCandidates()
	if err != nil {
		return "", err
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

func defaultConfigPath() (string, error) {
	candidates, err := defaultConfigCandidates()
	if err != nil {
		return "", err
	}
	return candidates[0], nil
}

func defaultConfigCandidates() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	return []string{
		filepath.Join(home, ".config", "igvprox", "config.toml"),
		filepath.Join(home, ".igvproxrc"),
	}, nil
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

func defaultConfig() Config {
	return Config{
		Genome:            "hg38",
		BrowserURL:        "http://localhost:5000",
		AllowMissingIndex: false,
		ConstantTracks: []Track{
			{
				Name:     "Refseq Select",
				URL:      "https://hgdownload.soe.ucsc.edu/goldenPath/hg38/database/ncbiRefSeqSelect.txt.gz",
				Format:   "refgene",
				Type:     "annotation",
				Genome:   "hg38",
				Enabled:  true,
			},
		},
	}
}

func writeDefaultConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create config %q: %w", path, err)
	}
	defer file.Close()

	var b strings.Builder
	b.WriteString("# igvprox config\n")
	b.WriteString("# Auto-generated on first run.\n\n")
	b.WriteString(fmt.Sprintf("genome = %q\n", cfg.Genome))
	b.WriteString(fmt.Sprintf("browser_url = %q\n", cfg.BrowserURL))
	b.WriteString("socket_path = \"\"\n")
	b.WriteString(fmt.Sprintf("allow_missing_index = %t\n", cfg.AllowMissingIndex))
	b.WriteString("\n")
	b.WriteString("# Constant tracks are always available in the web UI.\n")
	b.WriteString("# They can be enabled or disabled from the igvprox menu.\n")
	for _, track := range cfg.ConstantTracks {
		b.WriteString("\n[[constant_track]]\n")
		b.WriteString(fmt.Sprintf("name = %q\n", track.Name))
		b.WriteString(fmt.Sprintf("url = %q\n", track.URL))
		if track.IndexURL != "" {
			b.WriteString(fmt.Sprintf("index_url = %q\n", track.IndexURL))
		}
		b.WriteString(fmt.Sprintf("format = %q\n", track.Format))
		b.WriteString(fmt.Sprintf("type = %q\n", track.Type))
		b.WriteString(fmt.Sprintf("genome = %q\n", track.Genome))
		b.WriteString(fmt.Sprintf("enabled = %t\n", track.Enabled))
	}

	if _, err := file.WriteString(b.String()); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
}

func TrackID(track Track) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		track.Name,
		track.URL,
		track.IndexURL,
		track.Format,
		track.Type,
		track.Genome,
	}, "|")))
	return hex.EncodeToString(sum[:])
}
