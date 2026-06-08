package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
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

const certHookTemplate = `#!/bin/sh
# Généré par deployeur setup : copie le cert TLS vers un emplacement lisible
# par l'user du daemon, à chaque renouvellement.
[ "$RENEWED_LINEAGE" = "/etc/letsencrypt/live/%[1]s" ] || exit 0
install -d -m 750 -o %[2]s -g %[2]s %[3]s
install -m 640 -o %[2]s -g %[2]s "$RENEWED_LINEAGE/fullchain.pem" %[3]s/fullchain.pem
install -m 640 -o %[2]s -g %[2]s "$RENEWED_LINEAGE/privkey.pem"   %[3]s/privkey.pem
systemctl try-reload-or-restart deployeur-webhook.service 2>/dev/null || true
`

const certHookPath = "/etc/letsencrypt/renewal-hooks/deploy/deployeur.sh"

// setup prepares the server to run under an existing user: dirs, systemd
// service, sudoers, firewall, TLS. With dryRun it only prints the actions.
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

	// Config serveur : hostname (FQDN), port, user.
	g, _, err := loadGlobal()
	if err != nil {
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
		configureNotify(&g)
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
		{fmt.Sprintf("ouvrir le port %d dans le pare-feu", g.Port), func() error {
			return openFirewall(g.Port)
		}},
		{fmt.Sprintf("TLS pour %s (cert + hook lisible par %s)", g.Hostname, runUser), func() error {
			return setupTLS(&g)
		}},
		{"écrire la config serveur " + globalPath(), func() error {
			return saveGlobal(g)
		}},
		{"activer et démarrer le service", func() error {
			if err := sh("systemctl", "daemon-reload"); err != nil {
				return err
			}
			return sh("systemctl", "enable", "--now", "deployeur-webhook.service")
		}},
	}

	header("Préparation du serveur")
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

	printPostSetup(g, u.HomeDir)
	return nil
}

// openFirewall opens the port if a host firewall is active. An inactive ufw is
// left untouched (enabling it could cut SSH).
func openFirewall(port int) error {
	rule := fmt.Sprintf("%d/tcp", port)
	if _, err := exec.LookPath("ufw"); err == nil {
		out, _ := exec.Command("ufw", "status").CombinedOutput()
		switch {
		case !strings.Contains(string(out), "Status: active"):
			fmt.Println("  ufw inactif → port déjà joignable localement (vérifie un éventuel pare-feu OVH)")
		case strings.Contains(string(out), rule):
			fmt.Println("  port déjà autorisé (ufw)")
		default:
			return sh("ufw", "allow", rule)
		}
		return nil
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		if err := sh("firewall-cmd", "--permanent", "--add-port="+rule); err != nil {
			return err
		}
		return sh("firewall-cmd", "--reload")
	}
	fmt.Println("  aucun pare-feu local (ufw/firewalld) → port joignable localement")
	return nil
}

// setupTLS installs the certbot deploy hook and, if a cert already exists for
// the hostname, copies it to a user-readable location and points the config at
// it. If no cert exists, it prints how to obtain one.
func setupTLS(g *Global) error {
	dst := filepath.Join(etcDir, "tls")
	live := "/etc/letsencrypt/live/" + g.Hostname

	hook := fmt.Sprintf(certHookTemplate, g.Hostname, g.User, dst)
	if err := os.MkdirAll(filepath.Dir(certHookPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(certHookPath, []byte(hook), 0o755); err != nil {
		return err
	}

	if !exists(live + "/fullchain.pem") {
		if err := obtainCert(g.Hostname); err != nil {
			fmt.Printf("  certbot a échoué (%v) — le daemon démarrera en HTTP ; corrige puis relance setup\n", err)
			return nil
		}
	}
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	for _, f := range []string{"fullchain.pem", "privkey.pem"} {
		if err := sh("install", "-m", "640", "-o", g.User, "-g", g.User, filepath.Join(live, f), filepath.Join(dst, f)); err != nil {
			return err
		}
	}
	g.TLSCert = filepath.Join(dst, "fullchain.pem")
	g.TLSKey = filepath.Join(dst, "privkey.pem")
	fmt.Printf("  cert copié dans %s, renouvellement géré par %s\n", dst, certHookPath)
	return nil
}

func printPostSetup(g Global, home string) {
	scheme := "https"
	if g.TLSCert == "" {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s:%d/hooks/<repo>", scheme, g.Hostname, g.Port)

	fmt.Println()
	box("Serveur prêt", []string{
		fmt.Sprintf("Daemon actif sous l'user « %s »", g.User),
		fmt.Sprintf("Webhook : %s", url),
		"",
		fmt.Sprintf("%s possède %s → « deployeur init » sans sudo.", g.User, etcDir),
	})
	box("À vérifier de ton côté", []string{
		fmt.Sprintf("• « %s » doit pouvoir « git fetch » dans chaque repo", g.User),
		fmt.Sprintf("  (clé ssh dans %s/.ssh ou token https) + droits", home),
		"  d'écriture sur les dossiers déployés, et — si PM2 —",
		"  être propriétaire des process pm2 (pm2 est par user).",
		fmt.Sprintf("• Pare-feu externe (OVH) éventuel : ouvrir le port %d.", g.Port),
	})
}

// obtainCert issues a Let's Encrypt cert for the hostname via certbot's Apache
// authenticator (non-interactive, relies on an already-registered account).
func obtainCert(hostname string) error {
	certbot := certbotPath()
	if certbot == "" {
		return fmt.Errorf("certbot introuvable — installe-le")
	}
	fmt.Printf("  obtention d'un cert Let's Encrypt pour %s (certbot --apache)…\n", hostname)
	return sh(certbot, "certonly", "--apache", "-d", hostname,
		"-n", "--agree-tos", "--keep-until-expiring")
}

func certbotPath() string {
	if p, err := exec.LookPath("certbot"); err == nil {
		return p
	}
	for _, p := range []string{"/snap/bin/certbot", "/usr/bin/certbot"} {
		if exists(p) {
			return p
		}
	}
	return ""
}

func sh(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
