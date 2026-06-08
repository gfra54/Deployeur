package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type deployer struct {
	dir    string
	repo   string
	branch string
	commit string
	env    []string
	log    io.Writer
}

// deploy runs the full deployment for the repo rooted at dir.
func deploy(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if !exists(filepath.Join(dir, ".git")) {
		return fmt.Errorf("%s n'est pas un dépôt git", dir)
	}

	repo := filepath.Base(dir)
	cfg, err := repoConfig(repo, dir)
	if err != nil {
		return fmt.Errorf("config %s: %w", repo, err)
	}
	branch := cfg.Branch
	if branch == "" {
		branch = gitDefaultBranch(dir)
	}

	lock, err := lockRepo(repo)
	if err != nil {
		return err
	}
	defer lock.Close()

	logw, closeLog := openLog(repo)
	defer closeLog()

	d := &deployer{dir: dir, repo: repo, branch: branch, log: logw}
	d.env = append(os.Environ(),
		"REPO="+repo,
		"BRANCH="+branch,
		"DEPLOY_DIR="+dir,
	)

	start := time.Now()
	d.logf("==> deploy %s (branche %s) — %s", repo, branch, start.Format(time.RFC3339))

	err = d.sync(cfg)
	dur := time.Since(start).Round(time.Millisecond)

	st := state{
		Repo:      repo,
		Branch:    branch,
		Commit:    d.commit,
		Timestamp: start.Format(time.RFC3339),
		Duration:  dur.String(),
		Success:   err == nil,
	}
	if err != nil {
		st.ExitCode = 1
		st.Error = err.Error()
		d.logf("==> ÉCHEC après %s: %v", dur, err)
	} else {
		d.logf("==> OK en %s (commit %s)", dur, short(d.commit))
	}
	saveState(st)
	if g, ok, gerr := loadGlobal(); gerr == nil && ok {
		notifyDeploy(g, st, d.logf)
	}
	return err
}

// sync updates the working tree (fast-forward only, never destructive) then
// runs before/steps/after. on_failure runs only if a phase started, not on a
// pre-flight refusal.
func (d *deployer) sync(cfg Config) error {
	// Pre-flight: no side effects on the app.
	if err := d.run("git fetch origin " + d.branch); err != nil {
		return err
	}
	if dirty := gitOut(d.dir, "status", "--porcelain", "--untracked-files=no"); dirty != "" {
		d.logf("modifications locales sur les fichiers suivis:\n%s", dirty)
		return fmt.Errorf("modifications locales non commitées, déploiement interrompu — aucune perte, résous le git status puis relance")
	}
	if err := d.run("git merge --ff-only origin/" + d.branch); err != nil {
		return fmt.Errorf("fast-forward impossible vers origin/%s (l'historique local a divergé): %w", d.branch, err)
	}
	d.commit = gitOut(d.dir, "rev-parse", "HEAD")
	d.env = append(d.env, "COMMIT="+d.commit)

	// Deploy phases: on_failure cleans up whatever before/steps may have started.
	for _, phase := range [][]string{cfg.Before, cfg.Steps, cfg.After} {
		for _, cmd := range phase {
			if err := d.run(cmd); err != nil {
				return d.fail(cfg, err)
			}
		}
	}
	return nil
}

// fail runs the on_failure hooks (errors there are logged, not propagated)
// and returns the original error.
func (d *deployer) fail(cfg Config, cause error) error {
	for _, cmd := range cfg.OnFailure {
		if err := d.run(cmd); err != nil {
			d.logf("on_failure: %q a échoué: %v", cmd, err)
		}
	}
	return cause
}

func (d *deployer) run(cmd string) error {
	d.logf("$ %s", cmd)
	c := exec.Command("sh", "-c", cmd)
	c.Dir = d.dir
	c.Env = d.env
	c.Stdout = d.log
	c.Stderr = d.log
	return c.Run()
}

func (d *deployer) logf(format string, a ...any) {
	fmt.Fprintf(d.log, format+"\n", a...)
}

// openLog returns a writer to stdout and, if writable, the repo log file.
func openLog(repo string) (io.Writer, func()) {
	f, err := os.OpenFile(filepath.Join(logDir, repo+".log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return os.Stdout, func() {}
	}
	return io.MultiWriter(os.Stdout, f), func() { f.Close() }
}

type state struct {
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	Timestamp string `json:"timestamp"`
	Duration  string `json:"duration"`
	ExitCode  int    `json:"exit_code"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func saveState(st state) {
	data, _ := json.MarshalIndent(st, "", "  ")
	os.WriteFile(filepath.Join(logDir, st.Repo+".state.json"), data, 0o640)
}

// readState loads a repo's last deploy state ({} if none yet).
func readState(repo string) state {
	var st state
	data, err := os.ReadFile(filepath.Join(logDir, repo+".state.json"))
	if err == nil {
		json.Unmarshal(data, &st)
	}
	return st
}

// gitOut runs a git command in dir and returns trimmed stdout ("" on error).
func gitOut(dir string, args ...string) string {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// targetBranch is the branch deployeur acts on for a repo: the configured
// branch, falling back to the checked-out branch.
func targetBranch(name, dir string) string {
	if cfg, err := repoConfig(name, dir); err == nil && cfg.Branch != "" {
		return cfg.Branch
	}
	return gitDefaultBranch(dir)
}

// gitDefaultBranch resolves the current branch, falling back to "master".
func gitDefaultBranch(dir string) string {
	if b := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD"); b != "" && b != "HEAD" {
		return b
	}
	return "master"
}

func short(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}
