package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSandboxSSHContractAndURLBinding(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	ca, _ := ssh.NewSignerFromKey(key)
	id := "sbx_0123456789abcdef01234567"
	host := SSHHost{HostCAPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(ca.PublicKey()))), HostKeyAlias: id + ".sandbox.runzivo"}
	var target string
	var mutate func(*SSHTicket)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sandboxPath("w", "/"+id+"/ssh-host") {
			_ = json.NewEncoder(w).Encode(host)
			return
		}
		if r.Method != "POST" || r.URL.Path != sandboxPath("w", "/"+id+"/ssh-ticket") {
			t.Error("wrong route")
		}
		var input map[string]string
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input["public_key"] != "key" {
			t.Error("missing public key")
		}
		response := SSHTicket{SSHHost: host, URL: target, Ticket: "secret-ticket", Principal: "user", ExpiresAt: time.Now().Add(30 * time.Second), CertificateExpiresAt: time.Now().Add(15 * time.Minute), Certificate: "certificate"}
		if mutate != nil {
			mutate(&response)
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	target = strings.Replace(c.baseURL, "http://", "ws://", 1) + sandboxPath("w", "/"+id+"/ssh")
	if _, e := c.SandboxSSHHost(context.Background(), "w", id); e != nil {
		t.Fatal(e)
	}
	if _, e := c.SandboxSSHTicket(context.Background(), "w", id, "key"); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*SSHTicket){
		func(v *SSHTicket) { v.URL = "wss://foreign.test" + sandboxPath("w", "/"+id+"/ssh") },
		func(v *SSHTicket) { v.URL += "?ticket=other" }, func(v *SSHTicket) { v.URL += "#fragment" },
		func(v *SSHTicket) { v.URL = strings.Replace(v.URL, "/w/", "/other/", 1) },
		func(v *SSHTicket) { v.Principal = "root" }, func(v *SSHTicket) { v.HostKeyAlias = "other.sandbox.runzivo" },
		func(v *SSHTicket) { v.HostCAPublicKey += "\nmalicious" }, func(v *SSHTicket) { v.ExpiresAt = time.Now().Add(-time.Second) },
		func(v *SSHTicket) { v.CertificateExpiresAt = time.Now().Add(time.Hour) },
	} {
		mutate = change
		if _, e := c.SandboxSSHTicket(context.Background(), "w", id, "key"); e == nil || strings.Contains(e.Error(), "secret-ticket") {
			t.Fatal("invalid or leaking response", e)
		}
	}
}
