package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// configFile is the legacy per-repo file. Configs now live centrally under
// repos.d/; this name is only used to read & migrate an old in-repo file.
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

// repoConfigDir is the central store holding every site's deploy config, one
// file per repo. It lives under etcDir (owned by the run user → writable by
// `init` without sudo).
func repoConfigDir() string          { return filepath.Join(etcDir, "repos.d") }
func repoConfigPath(n string) string { return filepath.Join(repoConfigDir(), n+".yml") }

// Config is a repo's deploy config, stored centrally at repos.d/<name>.yml.
type Config struct {
	Branch    string   `yaml:"branch"`
	Before    []string `yaml:"before,omitempty"`
	Steps     []string `yaml:"steps,omitempty"`
	After     []string `yaml:"after,omitempty"`
	OnFailure []string `yaml:"on_failure,omitempty"`
}

// loadConfig reads the central config for a repo by name (ok=false if none).
func loadConfig(name string) (Config, bool, error) {
	return readConfigFile(repoConfigPath(name))
}

// loadLegacyConfig reads an old in-repo .deployeur.yml, if one is still there.
func loadLegacyConfig(dir string) (Config, bool, error) {
	return readConfigFile(filepath.Join(dir, configFile))
}

func readConfigFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, false, nil
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

// saveConfig writes a repo's config to the central store.
func saveConfig(name string, c Config) error {
	if err := os.MkdirAll(repoConfigDir(), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(repoConfigPath(name), data, 0o640)
}

// repoConfig resolves a repo's effective config: the central store first, then
// a legacy in-repo .deployeur.yml (migrated on the next `init`), then
// auto-detection so deploy works on an unconfigured repo.
func repoConfig(name, dir string) (Config, error) {
	if c, ok, err := loadConfig(name); err != nil {
		return Config{}, err
	} else if ok {
		return c, nil
	}
	if c, ok, err := loadLegacyConfig(dir); err != nil {
		return Config{}, err
	} else if ok {
		return c, nil
	}
	return detect(dir), nil
}

// Repo is one entry in the global registry /etc/deployeur/repos.yml. It only
// maps a name to its directory + secret; the branch (and steps) live in the
// repo's .deployeur.yml, the single source of truth.
type Repo struct {
	Name   string `yaml:"name"`
	Dir    string `yaml:"dir"`
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
