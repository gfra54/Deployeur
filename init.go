package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// initRepo lets you view/edit a repo's deploy config from its directory, stores
// it centrally (repos.d/<name>.yml), registers the repo, and prints the webhook
// URL + HMAC secret to paste into the Git host.
func initRepo(yes bool) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if !exists(filepath.Join(dir, ".git")) {
		return fmt.Errorf("%s n'est pas un dépôt git (lance init à la racine du repo)", dir)
	}
	remote := gitOut(dir, "remote", "get-url", "origin")
	if remote == "" {
		return fmt.Errorf("aucun remote 'origin' configuré — deployeur suppose que `git fetch` fonctionne déjà")
	}

	g, ok, err := loadGlobal()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("serveur non préparé — lance d'abord `sudo deployeur setup`")
	}

	name := filepath.Base(dir)

	// Source de la config : store central existant > ancien .deployeur.yml
	// (migré) > auto-détection.
	cfg, haveCfg, err := loadConfig(name)
	if err != nil {
		return err
	}
	migrated := false
	if !haveCfg {
		if legacy, ok, lerr := loadLegacyConfig(dir); lerr == nil && ok {
			cfg, migrated = legacy, true
		} else {
			cfg = detect(dir)
		}
	}
	if cfg.Branch == "" {
		cfg.Branch = gitDefaultBranch(dir)
	}

	source := "proposée (auto-détectée)"
	switch {
	case haveCfg:
		source = "actuelle"
	case migrated:
		source = "reprise de l'ancien " + configFile
	}
	preview, _ := yaml.Marshal(cfg)
	fmt.Printf("Repo:    %s\nRemote:  %s\nBranche: %s\n\nConfig %s → %s :\n\n%s\n",
		name, remote, cfg.Branch, source, repoConfigPath(name), preview)

	if !yes {
		switch strings.ToLower(ask("Écrire ? [Y]es / [e]diter / [n]on", "Y")) {
		case "n", "no", "non":
			return fmt.Errorf("annulé")
		case "e", "edit", "editer", "éditer":
			if cfg, err = editConfig(cfg); err != nil {
				return err
			}
		}
	}

	if err := saveConfig(name, cfg); err != nil {
		return fmt.Errorf("écriture %s (droits ? lance setup d'abord): %w", repoConfigPath(name), err)
	}
	if migrated {
		if err := os.Remove(filepath.Join(dir, configFile)); err == nil {
			fmt.Printf("→ ancien %s supprimé, config désormais centralisée.\n", configFile)
		}
	}

	secret, err := register(name, dir)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s:%d/hooks/%s", g.Hostname, g.Port, name)
	fmt.Printf(`
✓ %s enregistré (config: %s).

  Webhook URL : %s
  Secret HMAC : %s

À coller côté GitHub (Settings → Webhooks) / GitLab / Gitea :
  - Payload URL  : l'URL ci-dessus
  - Content type : application/json
  - Secret       : le secret ci-dessus
  - Événements   : push uniquement

`, name, repoConfigPath(name), url, secret)
	return nil
}

// register adds or updates the repo in the registry, returning its HMAC secret
// (preserved across re-init so the Git host config stays valid).
func register(name, dir string) (string, error) {
	reg, err := loadRegistry()
	if err != nil {
		return "", err
	}
	for i := range reg.Repos {
		if reg.Repos[i].Name == name {
			reg.Repos[i].Dir = dir
			if reg.Repos[i].Secret == "" {
				reg.Repos[i].Secret = genSecret()
			}
			return reg.Repos[i].Secret, saveRegistry(reg)
		}
	}
	secret := genSecret()
	reg.Repos = append(reg.Repos, Repo{Name: name, Dir: dir, Secret: secret})
	if err := saveRegistry(reg); err != nil {
		return "", fmt.Errorf("écriture %s (droits insuffisants ? lance avec sudo ou en tant qu'user deployeur): %w", reposPath(), err)
	}
	return secret, nil
}

// editConfig opens the proposed config in $EDITOR and returns the edited result.
func editConfig(c Config) (Config, error) {
	f, err := os.CreateTemp("", "deployeur-*.yml")
	if err != nil {
		return c, err
	}
	defer os.Remove(f.Name())
	out, _ := yaml.Marshal(c)
	f.Write(out)
	f.Close()

	cmd := exec.Command(editorCmd(), f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return c, err
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return c, err
	}
	var nc Config
	return nc, yaml.Unmarshal(data, &nc)
}

// editorCmd picks the editor for config files: $EDITOR if set, otherwise nano
// when available, falling back to vi.
func editorCmd() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if p, err := exec.LookPath("nano"); err == nil {
		return p
	}
	return "vi"
}
