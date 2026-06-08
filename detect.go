package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	c.Steps = append(c.Steps, npmSteps(dir)...)
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

// npmSteps reads package.json and proposes the deploy-relevant npm scripts in a
// safe order: `npm ci` first (deps à jour), puis le build, et le test en
// dernier — ainsi un test qui échoue stoppe le déploiement avant le reload
// (en `after`) et déclenche la notification d'échec. Renvoie nil si le projet
// n'a ni build ni test exploitable.
func npmSteps(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}

	_, hasBuild := pkg.Scripts["build"]
	// Le `test` par défaut de npm est un placeholder qui échoue toujours
	// ("Error: no test specified") — on l'ignore pour ne pas casser le deploy.
	test, hasTest := pkg.Scripts["test"]
	hasTest = hasTest && !strings.Contains(test, "no test specified")

	if !hasBuild && !hasTest {
		return nil
	}
	steps := []string{"npm ci"}
	if hasBuild {
		steps = append(steps, "npm run build")
	}
	if hasTest {
		steps = append(steps, "npm test")
	}
	return steps
}
