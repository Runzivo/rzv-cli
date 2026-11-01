package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/oidc"
	"github.com/Runzivo/rzv-cli/internal/sshstate"
)

func (a *app) sshError(en, fr string) error {
	return newError("sandbox_ssh", 255, "%s", a.text(en, fr))
}

func sshSandboxID(host string) (string, error) {
	id := strings.TrimSuffix(host, ".rzv")
	id = strings.TrimSuffix(id, ".sandbox.runzivo")
	if host != id && host != id+".rzv" && host != id+".sandbox.runzivo" {
		return sandboxID(host)
	}
	return sandboxID(id)
}

func (a *app) sshState(cmd *command, id string) (*sshstate.State, *api.Client, error) {
	if _, err := sandboxID(id); err != nil {
		return nil, nil, err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return nil, nil, err
	}
	if a.Store == nil {
		return nil, nil, a.sshError("No private SSH state directory is available.", "Aucun dossier privé pour SSH n’est disponible.")
	}
	origin, identity := "", ""
	if key := a.apiKey(); key != "" {
		origin, _, err = a.apiKeyTarget()
		digest := sha256.Sum256([]byte(key))
		identity = "key:" + hex.EncodeToString(digest[:])
	} else {
		creds, e := a.credentials()
		if e != nil {
			return nil, nil, e
		}
		origin = creds.APIBaseURL
		claims, e := oidc.DecodeUnverified(creds.AccessToken)
		if e == nil && claims.String("sub") != "" {
			identity = creds.Issuer + "/" + claims.String("sub")
		} else {
			digest := sha256.Sum256([]byte(creds.AccessToken))
			identity = "session:" + hex.EncodeToString(digest[:])
		}
	}
	if err != nil {
		return nil, nil, err
	}
	binding := sshstate.Binding{Origin: origin, Identity: identity, Profile: a.profileName(), Workspace: workspace, Sandbox: id}
	expected, err := cmd.option("binding")
	if err != nil {
		return nil, nil, err
	}
	if expected != "" && expected != binding.ID() {
		return nil, nil, a.sshError("SSH configuration belongs to a different workspace or identity. Run ssh-config again.", "La configuration SSH appartient à un autre espace de travail ou à une autre identité. Relancez ssh-config.")
	}
	root, err := filepath.Abs(a.Store.Dir)
	if err != nil {
		return nil, nil, err
	}
	state, err := sshstate.Open(root, binding)
	return state, client, err
}

func pendingTicketPath(state *sshstate.State) string { return filepath.Join(state.Dir, "ticket.json") }

// Take a prepared ticket by an atomic rename. Concurrent connections never
// redeem the same file; losers request their own ticket. Claimed files are
// deleted even when validation or connection setup fails.
func takeSSHTicket(state *sshstate.State) (*api.SSHTicket, error) {
	f, err := os.CreateTemp(state.Dir, ".ticket-*")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	_ = f.Close()
	defer os.Remove(path)
	if err := os.Rename(pendingTicketPath(state), path); err != nil {
		return nil, err
	}
	data, err := sshstate.ReadPrivate(path)
	if err != nil {
		return nil, err
	}
	var ticket api.SSHTicket
	if json.Unmarshal(data, &ticket) != nil || !ticket.ExpiresAt.After(time.Now().Add(2*time.Second)) {
		return nil, errors.New("expired SSH ticket")
	}
	return &ticket, nil
}

func (a *app) prepareSSH(state *sshstate.State, client *api.Client) (*api.SSHTicket, error) {
	ticket, err := client.SandboxSSHTicket(a.ctx, state.Binding.Workspace, state.Binding.Sandbox, state.PublicKey())
	if err != nil {
		return nil, apiError(err, "")
	}
	if err := state.SaveCertificate(ticket.Certificate, ticket.CertificateExpiresAt, a.Now()); err != nil {
		return nil, err
	}
	if err := state.SaveHost(ticket.HostKeyAlias, ticket.HostCAPublicKey); err != nil {
		return nil, err
	}
	return ticket, nil
}

func sandboxOpenSSH(a *app, cmd *command) error {
	if runtime.GOOS == "windows" {
		return a.sshError("Sandbox OpenSSH currently requires macOS or Linux. Use sandboxes exec on Windows.", "OpenSSH pour les bacs à sable nécessite actuellement macOS ou Linux. Utilisez sandboxes exec sous Windows.")
	}
	if err := cmd.allow("workspace", "account", "stdio", "binding"); err != nil {
		return err
	}
	if cmd.json() {
		return usageError("%s", a.text("--json is not valid for SSH.", "--json n’est pas valide pour SSH."))
	}
	if len(cmd.args) < 2 {
		return usageError("%s", a.text("Use rzv sandboxes ssh <sandbox> [-- <ssh arguments>].", "Utilisez rzv sandboxes ssh <sandbox> [-- <arguments SSH>]."))
	}
	id, err := sshSandboxID(cmd.args[1])
	if err != nil {
		return err
	}
	if cmd.flags["stdio"] || cmd.args[0] == "ssh-proxy" {
		if len(cmd.args) != 2 {
			return usageError("%s", a.text("Use rzv sandboxes ssh <sandbox> --stdio.", "Utilisez rzv sandboxes ssh <sandbox> --stdio."))
		}
		state, client, err := a.sshState(cmd, id)
		if err != nil {
			return err
		}
		ticket, err := takeSSHTicket(state)
		if err != nil {
			ticket, err = a.prepareSSH(state, client)
		} else {
			// Revalidate persisted data before using it; no persisted field controls
			// the destination except through the API client's strict URL check.
			if ticket.Principal != "user" || ticket.SSHHost.Validate(id) != nil {
				err = errors.New("invalid SSH ticket")
			}
			if err == nil {
				err = state.SaveCertificate(ticket.Certificate, ticket.CertificateExpiresAt, a.Now())
			}
			if err == nil {
				err = state.SaveHost(ticket.HostKeyAlias, ticket.HostCAPublicKey)
			}
		}
		if err != nil {
			return err
		}
		socket, err := client.SandboxSSH(a.ctx, state.Binding.Workspace, id, ticket)
		if err != nil {
			return apiError(err, "")
		}
		if a.Stdin == nil {
			_ = socket.Abort()
			return a.sshError("SSH proxy needs standard input.", "Le relais SSH nécessite l’entrée standard.")
		}
		if err := proxySSH(a.ctx, socket, a.Stdin, a.Stdout); err != nil {
			return a.sshError("The SSH relay connection ended unexpectedly.", "La connexion au relais SSH s’est terminée de façon inattendue.")
		}
		return nil
	}
	options, remote, err := a.splitSSHArguments(cmd.args[2:])
	if err != nil {
		return err
	}
	path, err := exec.LookPath("ssh")
	if err != nil {
		if len(cmd.args) > 2 {
			return a.sshError("Install OpenSSH to run SSH commands. Use sandboxes terminal for a PTY shell.", "Installez OpenSSH pour exécuter des commandes SSH. Utilisez sandboxes terminal pour un shell PTY.")
		}
		return sandboxesTerminal(a, cmd)
	}
	state, client, err := a.sshState(cmd, id)
	if err != nil {
		return err
	}
	ticket, err := a.prepareSSH(state, client)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(ticket)
	if err := sshstate.WritePrivate(pendingTicketPath(state), data); err != nil {
		return err
	}
	config, err := a.sshConfig(state)
	if err != nil {
		return err
	}
	configPath := filepath.Join(state.Dir, "config")
	if err := sshstate.WritePrivate(configPath, []byte(config)); err != nil {
		return err
	}
	// OpenSSH options precede its destination; a remote command follows it.
	args := append([]string{"-F", configPath}, options...)
	args = append(args, id+".rzv")
	args = append(args, remote...)
	defer os.Remove(pendingTicketPath(state))
	return runOpenSSH(path, args)
}

// Forward only options with unambiguous arity. Security-critical config options
// are generated by rzv; remote commands remain arbitrary OpenSSH command text.
func (a *app) splitSSHArguments(args []string) ([]string, []string, error) {
	var options []string
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			break
		}
		switch arg {
		case "-t", "-T", "-tt", "-N", "-n", "-v", "-vv", "-vvv":
			options = append(options, arg)
		case "-L", "-l":
			if len(args) == 0 || args[0] == "" {
				return nil, nil, usageError(a.text("%s needs a value.", "%s nécessite une valeur."), arg)
			}
			options = append(options, arg, args[0])
			args = args[1:]
		default:
			if strings.HasPrefix(arg, "-L") && len(arg) > 2 {
				options = append(options, arg)
			} else {
				return nil, nil, usageError(a.text("Unsupported SSH option %s. Use the generated ssh-config for other OpenSSH options.", "Option SSH non prise en charge : %s. Utilisez la configuration ssh-config générée pour les autres options OpenSSH."), display(arg))
			}
		}
	}
	return options, args, nil
}

func shellArg(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }
func configPath(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid SSH configuration path")
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(value) + `"`, nil
}

func (a *app) sshConfig(state *sshstate.State) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	// OpenSSH expands percent tokens before invoking the user's shell.
	prefix := []string{"env", "RZV_CONFIG_DIR=" + filepath.Dir(filepath.Dir(state.Dir))}
	command := append(append([]string{}, prefix...), []string{executable, "--profile", state.Binding.Profile, "sandboxes", "ssh", "--stdio", "%h", "--workspace", state.Binding.Workspace, "--binding", state.Binding.ID()}...)
	refresh := append(append([]string{}, prefix...), []string{executable, "--profile", state.Binding.Profile, "sandboxes", "ssh-config", state.Binding.Sandbox, "--refresh", "--workspace", state.Binding.Workspace, "--binding", state.Binding.ID()}...)
	quote := func(args []string) (string, error) {
		for i, arg := range args {
			if strings.ContainsAny(arg, "\n\r\x00") {
				return "", errors.New("invalid SSH argument")
			}
			if arg != "%h" {
				arg = strings.ReplaceAll(arg, "%", "%%")
			}
			args[i] = shellArg(arg)
		}
		return strings.Join(args, " "), nil
	}
	proxy, err := quote(command)
	if err != nil {
		return "", err
	}
	preflight, err := quote(refresh)
	if err != nil {
		return "", err
	}
	// Match exec needs a quoted config argument around the shell command.
	preflight = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(preflight) + `"`
	identity, err := configPath(state.IdentityFile())
	if err != nil {
		return "", err
	}
	cert, err := configPath(state.CertificateFile())
	if err != nil {
		return "", err
	}
	hosts, err := configPath(state.KnownHostsFile())
	if err != nil {
		return "", err
	}
	alias := state.Binding.Sandbox + ".rzv"
	return fmt.Sprintf(`# Managed by rzv. Regenerate with rzv sandboxes ssh-config --write.
Host %s
  HostName %s
  User user
  HostKeyAlias %s.sandbox.runzivo
  IdentityFile %s
  CertificateFile %s
  UserKnownHostsFile %s
  GlobalKnownHostsFile /dev/null
  StrictHostKeyChecking yes
  IdentitiesOnly yes
  PreferredAuthentications publickey
  PasswordAuthentication no
  KbdInteractiveAuthentication no
  ForwardAgent no
  ForwardX11 no
  ServerAliveInterval 30
  ServerAliveCountMax 3
  ProxyCommand %s
Match host %s exec %s
Host *
`, alias, alias, state.Binding.Sandbox, identity, cert, hosts, proxy, alias, preflight), nil
}

func sandboxSSHConfig(a *app, cmd *command) error {
	if runtime.GOOS == "windows" {
		return a.sshError("Sandbox SSH configuration currently requires macOS or Linux.", "La configuration SSH des bacs à sable nécessite actuellement macOS ou Linux.")
	}
	if err := cmd.allow("workspace", "account", "write", "refresh", "binding"); err != nil {
		return err
	}
	if cmd.json() || len(cmd.args) > 2 {
		return usageError("%s", a.text("Use rzv sandboxes ssh-config [<sandbox>] [--write].", "Utilisez rzv sandboxes ssh-config [<sandbox>] [--write]."))
	}
	if cmd.flags["refresh"] {
		if len(cmd.args) != 2 || cmd.flags["write"] {
			return usageError("%s", a.text("SSH refresh needs one sandbox.", "L’actualisation SSH nécessite un seul bac à sable."))
		}
		id, err := sshSandboxID(cmd.args[1])
		if err != nil {
			return err
		}
		state, client, err := a.sshState(cmd, id)
		if err != nil {
			return err
		}
		// A wrapper may have prepared this connection before exec. Otherwise every
		// config evaluation refreshes before OpenSSH loads its certificate file.
		if data, e := sshstate.ReadPrivate(pendingTicketPath(state)); e == nil {
			var ticket api.SSHTicket
			if json.Unmarshal(data, &ticket) == nil && ticket.ExpiresAt.After(a.Now().Add(2*time.Second)) {
				return nil
			}
		}
		ticket, err := a.prepareSSH(state, client)
		if err != nil {
			return err
		}
		data, _ := json.Marshal(ticket)
		return sshstate.WritePrivate(pendingTicketPath(state), data)
	}
	var ids []string
	if len(cmd.args) == 2 {
		id, err := sshSandboxID(cmd.args[1])
		if err != nil {
			return err
		}
		ids = []string{id}
	} else {
		ws, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		listing, err := client.SandboxList(a.ctx, ws)
		if err != nil {
			return apiError(err, "")
		}
		for _, sandbox := range listing.Sandboxes {
			if sandbox.Status == "running" {
				ids = append(ids, sandbox.ID)
			}
		}
	}
	for _, id := range ids {
		state, client, err := a.sshState(cmd, id)
		if err != nil {
			return err
		}
		host, err := client.SandboxSSHHost(a.ctx, state.Binding.Workspace, id)
		if err != nil {
			return apiError(err, "")
		}
		if err := state.SaveHost(host.HostKeyAlias, host.HostCAPublicKey); err != nil {
			return err
		}
		config, err := a.sshConfig(state)
		if err != nil {
			return err
		}
		if cmd.flags["write"] {
			if err := a.writeSSHConfig(id, config); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(a.Stdout, config); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *app) writeSSHConfig(id, config string) error {
	home := a.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	dir := filepath.Join(home, ".ssh")
	if err := sshstate.PrivateDir(dir); err != nil {
		return err
	}
	owned := filepath.Join(dir, "rzv")
	if err := sshstate.PrivateDir(owned); err != nil {
		return err
	}
	path := filepath.Join(owned, id+".conf")
	if data, err := sshstate.ReadPrivate(path); err == nil {
		if !strings.HasPrefix(string(data), "# Managed by rzv.") {
			return a.sshError("Refusing to overwrite an SSH config not owned by rzv.", "Refus de remplacer une configuration SSH qui n’appartient pas à rzv.")
		}
	} else if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		return err
	}
	if err := sshstate.WritePrivate(path, []byte(config)); err != nil {
		return err
	}
	userConfig := filepath.Join(dir, "config")
	if info, err := os.Lstat(userConfig); err == nil && !info.Mode().IsRegular() {
		return sshstate.ErrState
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(userConfig)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	include := "Include ~/.ssh/rzv/*.conf\n"
	if !strings.HasPrefix(string(data), include) {
		if err := sshstate.WritePrivate(userConfig, append([]byte(include), data...)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(a.Stdout, a.text("Updated SSH configuration for ", "Configuration SSH mise à jour pour ")+id+".rzv")
	return err
}
