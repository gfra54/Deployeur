package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// Notify est la config de notification par serveur (dans
// /etc/deployeur/config.yml). Mattermost est prévenu à chaque déploiement ;
// l'email n'est envoyé qu'en cas d'échec.
type Notify struct {
	MattermostURL string `yaml:"mattermost_url,omitempty"`
	SMTP          SMTP   `yaml:"smtp,omitempty"`
}

// SMTP est la config d'envoi mail pour les alertes d'échec. L'auth est
// facultative (user/pass vides = relais sans auth). STARTTLS est utilisé si le
// serveur le propose (cas classique des ports 587/25).
type SMTP struct {
	Host string   `yaml:"host,omitempty"`
	Port int      `yaml:"port,omitempty"`
	User string   `yaml:"user,omitempty"`
	Pass string   `yaml:"pass,omitempty"`
	From string   `yaml:"from,omitempty"`
	To   []string `yaml:"to,omitempty"`
}

// notifyDeploy envoie les notifications post-déploiement : Mattermost à chaque
// fois, plus un email (et une mention @all sur Mattermost) en cas d'échec. Les
// erreurs sont loguées via logf, jamais propagées — une notif ratée ne doit pas
// faire échouer le déploiement.
func notifyDeploy(g Global, st state, logf func(string, ...any)) {
	n := g.Notify
	if st.Success {
		msg := fmt.Sprintf(":white_check_mark: **%s** déployé sur `%s` — branche `%s`, commit `%s`, en %s",
			st.Repo, g.Hostname, st.Branch, short(st.Commit), st.Duration)
		if err := postMattermost(n.MattermostURL, msg); err != nil {
			logf("notif mattermost: %v", err)
		}
		return
	}

	// Échec : Mattermost avec @all + email.
	msg := fmt.Sprintf("@all :red_circle: **Échec du déploiement de %s** sur `%s` (branche `%s`)\n```\n%s\n```",
		st.Repo, g.Hostname, st.Branch, st.Error)
	if err := postMattermost(n.MattermostURL, msg); err != nil {
		logf("notif mattermost: %v", err)
	}

	subject := fmt.Sprintf("[deployeur] ÉCHEC %s sur %s", st.Repo, g.Hostname)
	body := fmt.Sprintf(`Le déploiement de %s a échoué.

Serveur : %s
Branche : %s
Commit  : %s
Heure   : %s
Durée   : %s

Erreur :
%s

Détail : deployeur logs %s
`, st.Repo, g.Hostname, st.Branch, st.Commit, st.Timestamp, st.Duration, st.Error, st.Repo)
	if err := sendEmail(n.SMTP, subject, body); err != nil {
		logf("notif email: %v", err)
	}
}

// configureNotify demande interactivement la config Mattermost + email et la
// stocke dans g. Les valeurs existantes servent de défaut, de sorte qu'un
// nouveau passage de setup les conserve (entrée vide = on garde).
func configureNotify(g *Global) {
	n := &g.Notify
	fmt.Println("\nNotifications de déploiement :")

	if askYesNo("  Activer Mattermost (notif à chaque déploiement) ?", n.MattermostURL != "") {
		n.MattermostURL = ask("    URL du webhook Mattermost", n.MattermostURL)
		for n.MattermostURL != "" && askYesNo("    Envoyer un message de test maintenant ?", true) {
			msg := fmt.Sprintf(":satellite: deployeur — test de notification depuis `%s`", g.Hostname)
			if err := postMattermost(n.MattermostURL, msg); err != nil {
				fmt.Printf("    échec : %v\n", err)
				if !askYesNo("    Corriger l'URL ?", true) {
					break
				}
				n.MattermostURL = ask("    URL du webhook Mattermost", n.MattermostURL)
			} else {
				fmt.Println("    envoyé ✓ — vérifie le canal Mattermost")
				break
			}
		}
	} else {
		n.MattermostURL = ""
	}

	if askYesNo("  Activer les alertes email (en cas d'échec) ?", n.SMTP.Host != "") {
		s := &n.SMTP
		askSMTP(s, g.Hostname)
		for s.Host != "" && len(s.To) > 0 && askYesNo("    Envoyer un email de test maintenant ?", true) {
			subject := "[deployeur] email de test depuis " + g.Hostname
			body := fmt.Sprintf("Ceci est un email de test envoyé par deployeur setup depuis %s.\nSi tu le reçois, les alertes d'échec fonctionnent.\n", g.Hostname)
			if err := sendEmail(*s, subject, body); err != nil {
				fmt.Printf("    échec : %v\n", err)
				if !askYesNo("    Reprendre la config SMTP ?", true) {
					break
				}
				askSMTP(s, g.Hostname)
			} else {
				fmt.Printf("    envoyé ✓ — vérifie la boîte %s\n", strings.Join(s.To, ", "))
				break
			}
		}
	} else {
		n.SMTP = SMTP{}
	}
}

// askSMTP demande (ou re-demande) les paramètres SMTP, en proposant les valeurs
// déjà saisies comme défauts.
func askSMTP(s *SMTP, hostname string) {
	s.Host = ask("    Serveur SMTP (host)", s.Host)
	s.Port = askInt("    Port SMTP", orInt(s.Port, 587))
	s.User = ask("    Utilisateur SMTP (vide = sans auth)", s.User)
	if s.User != "" {
		s.Pass = ask("    Mot de passe SMTP", s.Pass)
	} else {
		s.Pass = ""
	}
	s.From = ask("    Expéditeur (From)", orStr(s.From, "deployeur@"+hostname))
	s.To = splitList(ask("    Destinataire(s), séparés par des virgules", strings.Join(s.To, ", ")))
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// splitList parse une liste séparée par des virgules en ignorant les blancs.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// postMattermost poste un message sur une URL d'incoming webhook. Une URL vide
// est un no-op (le canal n'est simplement pas configuré).
func postMattermost(url, text string) error {
	if url == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("mattermost a répondu %s", resp.Status)
	}
	return nil
}

// sendEmail envoie un message texte via le serveur SMTP configuré. Un host vide
// ou une liste de destinataires vide est un no-op.
func sendEmail(s SMTP, subject, body string) error {
	if s.Host == "" || len(s.To) == 0 {
		return nil
	}
	port := s.Port
	if port == 0 {
		port = 587
	}
	from := s.From
	if from == "" {
		from = "deployeur@" + s.Host
	}
	addr := fmt.Sprintf("%s:%d", s.Host, port)

	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}
	return smtp.SendMail(addr, auth, from, s.To, buildMessage(from, s.To, subject, body))
}

// buildMessage assemble un mail RFC 5322 minimal (UTF-8, texte brut).
func buildMessage(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(b.String())
}
