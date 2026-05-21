package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configFile = ".deployeur.yml"

// Base directories, overridable via env for tests (unset in production).
var (
	etcDir = envOr("DEPLOYEUR_ETC", "/etc/deployeur")
	logDir = envOr("DEPLOYEUR_LOG", "/var/log/deployeur")
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func reposPath() string  { return filepath.Join(etcDir, "repos.yml") }
func globalPath() string { return filepath.Join(etcDir, "config.yml") }

// Config is the per-repo .deployeur.yml.
type Config struct {
	Branch    string   `yaml:"branch"`
	Before    []string `yaml:"before,omitempty"`
	Steps     []string `yaml:"steps,omitempty"`
	After     []string `yaml:"after,omitempty"`
	OnFailure []string `yaml:"on_failure,omitempty"`
}

// loadConfig reads .deployeur.yml from dir. If absent, it falls back to the
// auto-detected defaults so `deploy` works on an unconfigured repo.
func loadConfig(dir string) (Config, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if os.IsNotExist(err) {
		return detect(dir), false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

// Repo is one entry in the global registry /etc/deployeur/repos.yml.
type Repo struct {
	Name   string `yaml:"name"`
	Dir    string `yaml:"dir"`
	Branch string `yaml:"branch"`
	Secret string `yaml:"secret"`
}

type Registry struct {
	Repos []Repo `yaml:"repos"`
}

func loadRegistry() (Registry, error) {
	var r Registry
	data, err := os.ReadFile(reposPath())
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	return r, yaml.Unmarshal(data, &r)
}

func saveRegistry(r Registry) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(reposPath(), data, 0o640)
}
