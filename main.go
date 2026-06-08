package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[2:]
	var err error

	switch os.Args[1] {
	case "deploy":
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		err = deploy(dir)
	case "init":
		err = initRepo(hasFlag(args, "-y", "--yes"))
	case "webhook":
		err = runWebhook()
	case "status":
		err = status()
	case "logs", "log":
		err = logs(firstArg(args), lastN(args))
	case "setup":
		err = setup(flagVal(args, "--user"), hasFlag(args, "--dry-run", "-n"))
	case "version", "-v", "--version":
		fmt.Printf("deployeur %s (%s, build %s)\n", version, commit, buildDate)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue: %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
}

func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n {
				return true
			}
		}
	}
	return false
}

// flagVal returns the value following a `--flag value` argument, or "".
func flagVal(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// firstArg returns the first positional (non-flag) argument, skipping `--last`
// and the optional number that follows it.
func firstArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--last" {
			if i+1 < len(args) {
				if _, err := strconv.Atoi(args[i+1]); err == nil {
					i++
				}
			}
			continue
		}
		if !strings.HasPrefix(args[i], "-") {
			return args[i]
		}
	}
	return ""
}

// lastN parses `--last [N]`: 0 means follow (no --last), N>0 prints N lines
// (default 200 if --last is given without a number).
func lastN(args []string) int {
	for i, a := range args {
		if a != "--last" {
			continue
		}
		if i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				return n
			}
		}
		return 200
	}
	return 0
}

func usage() {
	fmt.Print(`deployeur — déploiement auto-hébergé

usage: deployeur <commande> [args]

commandes:
  deploy [dir]   déploie le repo (dossier courant par défaut)
  init [-y]      voir/éditer la conf de déploiement du repo (stockée au central), enregistre le webhook
  webhook        lance le daemon (TLS sur le port dédié + admin local 127.0.0.1:9000)
  setup [-n]     prépare le serveur (dossiers, systemd, sudoers) sous un user existant
                 (--user <nom>, défaut $SUDO_USER) — root requis, -n=dry-run
  status         tableau d'état de tous les repos enregistrés
  log [repo]     suit les logs en temps réel — tous les repos, ou un seul si précisé
                 (alias: logs ; --last [N] pour les N dernières lignes au lieu du suivi)
  version        affiche la version
`)
}
