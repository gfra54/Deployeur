package main

import (
	"fmt"
	"os"
)

var version = "dev"

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
	case "init", "webhook", "setup", "status", "logs":
		err = fmt.Errorf("%q: pas encore implémenté", os.Args[1])
	case "version", "-v", "--version":
		fmt.Println("deployeur " + version)
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

func usage() {
	fmt.Print(`deployeur — déploiement auto-hébergé

usage: deployeur <commande> [args]

commandes:
  deploy [dir]   déploie le repo (dossier courant par défaut)
  init           scanne le repo, génère .deployeur.yml, enregistre le webhook
  webhook        daemon webhook / sous-commandes
  setup          prépare le serveur (user, dossiers, service systemd)
  status         état de tous les repos enregistrés
  logs <repo>    affiche les logs d'un repo
  version        affiche la version
`)
}
