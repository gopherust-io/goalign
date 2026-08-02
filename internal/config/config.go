// Package config loads .goalign.yml project defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileNames are searched from the start path upward.
var FileNames = []string{".goalign.yml", "goalign.yml"}

// Config holds optional defaults for analyze/fix.
type Config struct {
	Cacheguard     *bool    `yaml:"cacheguard"`
	CacheLine      *int     `yaml:"cache-line"`
	FailOnFindings *bool    `yaml:"fail-on-findings"`
	Recursive      *bool    `yaml:"recursive"`
	Packages       *bool    `yaml:"packages"`
	Jobs           *int     `yaml:"jobs"`
	RewriteBools   *bool    `yaml:"rewrite-bools"`
	MinWaste       *int     `yaml:"min-waste"`
	SkipGenerated  *bool    `yaml:"skip-generated"`
	Arch           string   `yaml:"arch"`
	Policy         string   `yaml:"policy"`
	Format         string   `yaml:"format"`
	Exclude        []string `yaml:"exclude"`
	Arches         []string `yaml:"arches"`
	IgnoreGlobs    []string `yaml:"ignore"`
	GeneratedGlobs []string `yaml:"generated"`
}

// Load walks from start (file or dir) toward filesystem root looking for a config file.
// Missing config is not an error; returns zero Config.
func Load(start string) (Config, string, error) {
	dir, err := absDir(start)
	if err != nil {
		return Config{}, "", err
	}
	for {
		for _, name := range FileNames {
			path := filepath.Join(dir, name)
			cfg, err := loadFile(path)
			if err == nil {
				return cfg, path, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return Config{}, path, err
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Config{}, "", nil
		}
		dir = parent
	}
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func absDir(start string) (string, error) {
	if start == "" {
		start = "."
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		// Path may not exist yet; still walk from parent intent.
		return filepath.Dir(abs), nil
	}
	if fi.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}
