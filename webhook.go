package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

const localAddr = "127.0.0.1:9000"

// daemon serializes deploys per repo with at-most-one-pending coalescing: a
// burst of webhooks collapses into a single re-deploy that fetches the latest.
type daemon struct {
	mu       sync.Mutex
	triggers map[string]chan struct{}
}

func runWebhook() error {
	g, ok, err := loadGlobal()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("config serveur absente (%s) — lance `deployeur init` ou `deployeur setup`", globalPath())
	}
	d := &daemon{triggers: map[string]chan struct{}{}}

	public := http.NewServeMux()
	public.HandleFunc("/hooks/", d.handleHook)

	local := http.NewServeMux()
	local.HandleFunc("/status", handleStatus)
	local.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "ok\n")
	})
	go func() {
		log.Printf("admin local sur http://%s (/status /healthz)", localAddr)
		log.Fatal(http.ListenAndServe(localAddr, local))
	}()

	addr := fmt.Sprintf(":%d", g.Port)
	if g.TLSCert != "" && g.TLSKey != "" {
		log.Printf("webhook TLS sur https://%s:%d/hooks/", g.Hostname, g.Port)
		return http.ListenAndServeTLS(addr, g.TLSCert, g.TLSKey, public)
	}
	log.Printf("webhook HTTP (sans TLS) sur %s — renseigne tls_cert/tls_key dans %s", addr, globalPath())
	return http.ListenAndServe(addr, public)
}

func (d *daemon) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/hooks/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	reg, err := loadRegistry()
	if err != nil {
		http.Error(w, "registry", http.StatusInternalServerError)
		return
	}
	repo, ok := findRepo(reg, name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		http.Error(w, "body", http.StatusBadRequest)
		return
	}
	if !validSig(repo.Secret, body, r.Header) {
		log.Printf("hook %s: signature invalide", name)
		http.Error(w, "signature invalide", http.StatusUnauthorized)
		return
	}

	var p struct {
		Ref string `json:"ref"`
	}
	json.Unmarshal(body, &p)
	branch := strings.TrimPrefix(p.Ref, "refs/heads/")
	switch {
	case branch == "":
		io.WriteString(w, "ok (pas de ref, ignoré)\n") // ex. ping GitHub
	case branch != repo.Branch:
		log.Printf("hook %s: branche %q ignorée (attendu %q)", name, branch, repo.Branch)
		fmt.Fprintf(w, "branche %q ignorée\n", branch)
	default:
		d.trigger(repo)
		log.Printf("hook %s: déploiement déclenché (%s)", name, branch)
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, "déploiement déclenché\n")
	}
}

// trigger enqueues a deploy for repo, coalescing if one is already pending.
func (d *daemon) trigger(repo Repo) {
	d.mu.Lock()
	ch, ok := d.triggers[repo.Name]
	if !ok {
		ch = make(chan struct{}, 1)
		d.triggers[repo.Name] = ch
		go worker(repo, ch)
	}
	d.mu.Unlock()

	select {
	case ch <- struct{}{}:
	default: // déjà en attente, on fusionne
	}
}

func worker(repo Repo, ch chan struct{}) {
	for range ch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("deploy %s: panic récupéré: %v", repo.Name, r)
				}
			}()
			if err := deploy(repo.Dir); err != nil {
				log.Printf("deploy %s: %v", repo.Name, err)
			}
		}()
	}
}

func handleStatus(w http.ResponseWriter, _ *http.Request) {
	reg, _ := loadRegistry()
	out := make([]state, 0, len(reg.Repos))
	for _, repo := range reg.Repos {
		st := readState(repo.Name)
		st.Repo = repo.Name
		out = append(out, st)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func findRepo(reg Registry, name string) (Repo, bool) {
	for _, r := range reg.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

// validSig accepts GitHub/Gitea HMAC (X-Hub-Signature-256), Gitea's
// X-Gitea-Signature, or GitLab's plain X-Gitlab-Token.
func validSig(secret string, body []byte, h http.Header) bool {
	if sig := h.Get("X-Hub-Signature-256"); sig != "" {
		return hmacEq(secret, body, strings.TrimPrefix(sig, "sha256="))
	}
	if sig := h.Get("X-Gitea-Signature"); sig != "" {
		return hmacEq(secret, body, sig)
	}
	if tok := h.Get("X-Gitlab-Token"); tok != "" {
		return hmac.Equal([]byte(tok), []byte(secret))
	}
	return false
}

func hmacEq(secret string, body []byte, hexSig string) bool {
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}
