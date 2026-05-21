package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
)

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
# Autorise l'user %[1]s à recharger des services sans mot de passe.
# Décommente/adapte les lignes utiles :
# %[1]s ALL=(root) NOPASSWD: /usr/bin/systemctl reload apache2
# %[1]s ALL=(root) NOPASSWD: /usr/bin/systemctl reload php8.2-fpm
`

// setup prepares the server to run under an existing user: dirs, systemd
// service, sudoers template, per-server config. With dryRun it only prints.
func setup(runUser string, dryRun bool) error {
	if !dryRun && os.Geteuid() != 0 {
		return fmt.Errorf("setup doit tourner en root (sudo deployeur setup), ou utilise --dry-run")
	}
	if runUser == "" {
		runUser = os.Getenv("SUDO_USER")
	}
	if runUser == "" {
		return fmt.Errorf("impossible de déterminer l'utilisateur cible — précise-le avec --user <nom>")
	}
	u, err := user.Lookup(runUser)
	if err != nil {
		return fmt.Errorf("utilisateur %q introuvable (setup ne crée pas le user, choisis-en un existant): %w", runUser, err)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	owner := runUser + ":" + runUser

	steps := []struct {
		desc string
		fn   func() error
	}{
		{fmt.Sprintf("créer %s (chown %s)", etcDir, runUser), func() error {
			if err := os.MkdirAll(etcDir, 0o755); err != nil {
				return err
			}
			return sh("chown", "-R", owner, etcDir)
		}},
		{fmt.Sprintf("créer %s (0750, chown %s)", logDir, runUser), func() error {
			if err := os.MkdirAll(logDir, 0o750); err != nil {
				return err
			}
			return sh("chown", "-R", owner, logDir)
		}},
		{"installer /etc/systemd/system/deployeur-webhook.service", func() error {
			unit := fmt.Sprintf(unitTemplate, runUser, self)
			return os.WriteFile("/etc/systemd/system/deployeur-webhook.service", []byte(unit), 0o644)
		}},
		{"générer /etc/sudoers.d/deployeur (modèle à compléter)", func() error {
			content := fmt.Sprintf(sudoersTemplate, runUser)
			if err := os.WriteFile("/etc/sudoers.d/deployeur", []byte(content), 0o440); err != nil {
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

	g, _, err := loadGlobal()
	if err != nil && !dryRun {
		return err
	}
	if g.Hostname == "" {
		g.Hostname = defaultHostname()
	}
	if g.Port == 0 {
		if g.Port, err = pickPort(); err != nil {
			return err
		}
	}
	g.User = runUser
	if !dryRun {
		if err := saveGlobal(g); err != nil {
			return err
		}
	}
	printPostSetup(g, u.HomeDir)
	return nil
}

func printPostSetup(g Global, home string) {
	url := fmt.Sprintf("https://%s:%d/hooks/<repo>", g.Hostname, g.Port)
	fmt.Printf(`
Serveur prêt — daemon sous l'user %q. À finaliser de ton côté :

1. Pare-feu : ouvrir le port %d en entrée
     ufw allow %d/tcp

2. TLS pour %s :
   - soit un cert existant → renseigne tls_cert/tls_key dans %s
   - soit Let's Encrypt → %s
     (le daemon détecte ensuite /etc/letsencrypt/live/%s/ automatiquement)

3. Accès : l'user %q doit pouvoir faire `+"`git fetch`"+` dans chaque repo
   (deploy key ssh dans %s/.ssh, ou token https dans le remote), avoir les
   droits d'écriture sur les dossiers déployés, et — si tu utilises PM2 — être
   le propriétaire des process PM2 (pm2 est par utilisateur).

L'user possède %s, donc `+"`deployeur init`"+` tourne sans sudo dans chaque app.
Webhook annoncé : %s
`, g.User, g.Port, g.Port, g.Hostname, globalPath(), certbotHint(g.Hostname), g.Hostname, g.User, home, etcDir, url)
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
