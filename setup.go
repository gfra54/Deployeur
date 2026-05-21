package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
)

const deployUser = "deployeur"

const unitTemplate = `[Unit]
Description=deployeur webhook daemon
After=network.target

[Service]
Type=simple
User=%s
ExecStart=%s webhook
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const sudoersTemplate = `# Généré par deployeur setup — complète selon tes besoins, puis valide avec visudo.
# Autorise l'user deployeur à recharger des services sans mot de passe.
# Décommente/adapte les lignes utiles :
# deployeur ALL=(root) NOPASSWD: /usr/bin/systemctl reload apache2
# deployeur ALL=(root) NOPASSWD: /usr/bin/systemctl reload php8.2-fpm
`

// setup prepares the server: user, dirs, systemd service, sudoers template,
// per-server config. With dryRun it only prints the actions.
func setup(dryRun bool) error {
	if !dryRun && os.Geteuid() != 0 {
		return fmt.Errorf("setup doit tourner en root (sudo deployeur setup), ou utilise --dry-run")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}

	steps := []struct {
		desc string
		fn   func() error
	}{
		{fmt.Sprintf("créer l'utilisateur système %q (avec home pour ssh/git)", deployUser), func() error {
			if _, err := user.Lookup(deployUser); err == nil {
				fmt.Println("  déjà présent, ignoré")
				return nil
			}
			return sh("useradd", "--system", "--create-home", "--shell", "/bin/bash", deployUser)
		}},
		{"créer " + etcDir + " (0755)", func() error {
			return os.MkdirAll(etcDir, 0o755)
		}},
		{"créer " + logDir + " (0750, chown " + deployUser + ")", func() error {
			if err := os.MkdirAll(logDir, 0o750); err != nil {
				return err
			}
			return sh("chown", deployUser+":"+deployUser, logDir)
		}},
		{"installer /etc/systemd/system/deployeur-webhook.service", func() error {
			unit := fmt.Sprintf(unitTemplate, deployUser, self)
			return os.WriteFile("/etc/systemd/system/deployeur-webhook.service", []byte(unit), 0o644)
		}},
		{"générer /etc/sudoers.d/deployeur (modèle à compléter)", func() error {
			if err := os.WriteFile("/etc/sudoers.d/deployeur", []byte(sudoersTemplate), 0o440); err != nil {
				return err
			}
			return sh("visudo", "-cf", "/etc/sudoers.d/deployeur")
		}},
		{"activer et démarrer le service", func() error {
			if err := sh("systemctl", "daemon-reload"); err != nil {
				return err
			}
			return sh("systemctl", "enable", "--now", "deployeur-webhook.service")
		}},
	}

	for _, s := range steps {
		if dryRun {
			fmt.Println("[dry-run]", s.desc)
			continue
		}
		fmt.Println("•", s.desc)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.desc, err)
		}
	}

	g, err := ensureGlobal(true)
	if err != nil && !dryRun {
		return err
	}
	printPostSetup(g)
	return nil
}

func printPostSetup(g Global) {
	url := fmt.Sprintf("https://%s:%d/hooks/<repo>", g.Hostname, g.Port)
	fmt.Printf(`
Serveur prêt. À finaliser de ton côté :

1. Pare-feu : ouvrir le port %d en entrée
     ufw allow %d/tcp

2. TLS pour %s :
   - soit un cert existant → renseigne tls_cert/tls_key dans %s
   - soit Let's Encrypt → %s
     (le daemon détecte ensuite /etc/letsencrypt/live/%s/ automatiquement)

3. Accès git : l'user %q doit pouvoir faire `+"`git fetch`"+` dans chaque repo
   (deploy key ssh dans /home/%s/.ssh, ou token https dans le remote) et avoir
   les droits d'écriture sur les dossiers déployés.

Ensuite, dans chaque app : `+"`deployeur init`"+` → webhook %s
`, g.Port, g.Port, g.Hostname, globalPath(), certbotHint(g.Hostname), g.Hostname, deployUser, deployUser, url)
}

// certbotHint returns the right next step depending on certbot's presence.
func certbotHint(hostname string) string {
	if _, err := exec.LookPath("certbot"); err != nil {
		return "installe certbot (apt install certbot) puis: certbot certonly --standalone -d " + hostname
	}
	return "certbot certonly --standalone -d " + hostname + "  (port 80 libre requis le temps de la validation)"
}

func sh(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
