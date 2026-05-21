package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// detect inspects a repo directory and returns suggested deploy steps.
func detect(dir string) Config {
	var c Config
	name := filepath.Base(dir)

	if exists(filepath.Join(dir, "composer.json")) {
		c.Steps = append(c.Steps, "composer install --no-dev --optimize-autoloader")
	}
	if hasNpmBuild(dir) {
		c.Steps = append(c.Steps, "npm ci", "npm run build")
	}
	if exists(filepath.Join(dir, "artisan")) {
		c.Steps = append(c.Steps, "php artisan migrate --force", "php artisan config:cache")
	}
	if exists(filepath.Join(dir, "ecosystem.config.js")) {
		c.After = append(c.After, "pm2 reload "+name)
	}
	if exists(filepath.Join(dir, "wp-config.php")) {
		c.After = append(c.After, "wp cache flush")
	}
	return c
}

// hasNpmBuild reports whether package.json defines a "build" script.
func hasNpmBuild(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	_, ok := pkg.Scripts["build"]
	return ok
}
