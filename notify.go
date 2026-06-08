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
