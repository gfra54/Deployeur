package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand/v2"
	"net"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// Global is the per-server config /etc/deployeur/config.yml: how the webhook
// daemon is reached and where its TLS material lives.
type Global struct {
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	TLSCert  string `yaml:"tls_cert"`
	TLSKey   string `yaml:"tls_key"`
}

func loadGlobal() (Global, bool, error) {
	var g Global
	data, err := os.ReadFile(globalPath())
	if os.IsNotExist(err) {
		return g, false, nil
	}
	if err != nil {
		return g, false, err
	}
	return g, true, yaml.Unmarshal(data, &g)
}

func saveGlobal(g Global) error {
	data, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(globalPath(), data, 0o640)
}

// pickPort returns a random high port that is currently free to bind.
func pickPort() (int, error) {
	for range 100 {
		p := 20000 + mrand.IntN(40000)
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			l.Close()
			return p, nil
		}
	}
	return 0, fmt.Errorf("aucun port libre trouvé")
}

// genSecret returns a 256-bit hex secret for HMAC signing.
func genSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// defaultHostname returns the server's FQDN, falling back to the short name.
func defaultHostname() string {
	if out, err := exec.Command("hostname", "-f").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); h != "" {
			return h
		}
	}
	h, _ := os.Hostname()
	return h
}
