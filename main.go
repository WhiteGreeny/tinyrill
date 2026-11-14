package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/mythologyli/zju-connect/client/atrust/auth"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/internal/credentials"
	"github.com/mythologyli/zju-connect/internal/instance"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/state"
	"github.com/mythologyli/zju-connect/internal/terminal"
	"github.com/mythologyli/zju-connect/internal/tlspolicy"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/underlay"
	"golang.org/x/crypto/pkcs12"
)

var Version = "dev"
var Commit = "unknown"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `tinyrill — one customer VPN per process

  tinyrill run --config customer.yaml [--resources summary|full|none]
               [--resource-debug] [--resource-debug-values]
  tinyrill auth-info --config customer.yaml
  tinyrill config-check --config customer.yaml
  tinyrill credentials status|set|delete --config customer.yaml
  tinyrill session clear --config customer.yaml
  tinyrill version

Secrets are read from the terminal, an explicit secret file, or the selected
credential store. TOML, password flags and environment overrides are not accepted.
Exit codes: 0 stopped/success; 2 config/listener; 3 interaction required;
4 authentication rejected; 5 connection; 6 state/credential storage.
`

func runCLI(ctx context.Context, args []string, out, diagnostic io.Writer) int {
	log.Init()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return 0
	}
	command := args[0]
	args = args[1:]
	if command == "version" && len(args) == 0 {
		fmt.Fprintf(out, "tinyrill %s (%s) %s/%s %s\n", Version, Commit, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return 0
	}
	action := ""
	if command == "credentials" || command == "session" {
		if len(args) == 0 {
			return cliError(diagnostic, 2, errors.New("subcommand is required"))
		}
		action = args[0]
		args = args[1:]
	}
	switch command {
	case "run", "auth-info", "config-check":
	case "credentials":
		if action != "status" && action != "set" && action != "delete" {
			return cliError(diagnostic, 2, errors.New("unknown credentials subcommand"))
		}
	case "session":
		if action != "clear" {
			return cliError(diagnostic, 2, errors.New("unknown session subcommand"))
		}
	default:
		return cliError(diagnostic, 2, errors.New("unknown command; use tinyrill help"))
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "YAML configuration")
	resourceMode := "summary"
	debugResources := false
	debugResourceValues := false
	if command == "run" {
		flags.StringVar(&resourceMode, "resources", "summary", "authorized resource display: summary, full or none")
		flags.BoolVar(&debugResources, "resource-debug", false, "save resource field names and types, with values omitted")
		flags.BoolVar(&debugResourceValues, "resource-debug-values", false, "also save actual resource values to a private local file for manual review")
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 || *path == "" {
		return cliError(diagnostic, 2, errors.New("expected --config PATH; unknown arguments are not accepted"))
	}
	if resourceMode != "summary" && resourceMode != "full" && resourceMode != "none" {
		return cliError(diagnostic, 2, errors.New("--resources must be summary, full or none"))
	}
	p, err := configs.LoadProfile(*path)
	if err != nil {
		return cliError(diagnostic, 2, err)
	}
	if p.Debug {
		log.EnableDebug()
	} else {
		log.DisableDebug()
	}
	if _, err := tlspolicy.New(tlspolicy.Options{Mode: p.TLS.Mode, CAFile: p.TLS.CAFile, ServerName: p.TLS.ServerName, Pins: p.TLS.Pins}); err != nil {
		return cliError(diagnostic, 2, err)
	}
	if command == "config-check" {
		if err := p.ValidateForRun(); err != nil {
			return cliError(diagnostic, 2, err)
		}
		_ = json.NewEncoder(out).Encode(map[string]any{"schema_version": 1, "customer": p.ID, "valid": true})
		return 0
	}
	if command == "auth-info" {
		return authInfo(ctx, p, out, diagnostic)
	}
	if command == "session" {
		d, err := state.Acquire(p.Session.StateDir)
		if err != nil {
			return cliError(diagnostic, 6, err)
		}
		defer d.Close()
		if err = d.ClearSession(); err != nil {
			return cliError(diagnostic, 6, err)
		}
		fmt.Fprintln(out, `{"session":"cleared","remembered_password":"unchanged"}`)
		return 0
	}
	if err := p.ValidateForRun(); err != nil {
		return cliError(diagnostic, 2, err)
	}
	if command == "credentials" {
		return credentialCommand(ctx, p, action, out, diagnostic)
	}
	if p.TLS.Mode == "insecure" {
		fmt.Fprintln(diagnostic, "This profile explicitly disables VPN server identity verification.")
	}
	if p.EasyConnect.AllowLegacyTLS {
		fmt.Fprintln(diagnostic, "This profile explicitly permits the EC TLS 1.1/RC4 tunnel.")
	}
	material, err := loadMaterial(ctx, p, diagnostic)
	if err != nil {
		if errors.Is(err, terminal.ErrInteractionRequired) {
			return cliError(diagnostic, 3, err)
		}
		return cliError(diagnostic, 2, err)
	}
	frontend := &instance.CLIFrontend{Profile: p, Output: out, Diagnostic: diagnostic, ResourceMode: resourceMode, DebugResources: debugResources, DebugResourceValues: debugResourceValues}
	defer frontend.Close()
	err = instance.Run(ctx, p, material, frontend)
	if err == nil || ctx.Err() != nil || errors.Is(err, terminal.ErrCanceled) {
		return 0
	}
	var failure *instance.Failure
	if errors.As(err, &failure) {
		return cliError(diagnostic, failure.Code, err)
	}
	return cliError(diagnostic, 5, err)
}

func cliError(output io.Writer, code int, err error) int {
	if errors.Is(err, terminal.ErrCanceled) {
		return 0
	}
	fmt.Fprintln(output, terminal.SafeText(log.Redact(err.Error())))
	return code
}
func promptFor(p configs.Profile, output io.Writer) credentials.Prompt {
	return func(ctx context.Context, label string) (string, error) {
		if p.Auth.Interactive == "never" {
			return "", terminal.ErrInteractionRequired
		}
		timeout, _ := time.ParseDuration(p.Auth.Timeout)
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return terminal.ReadLine(ctx, output, label, true)
	}
}

func credentialCommand(ctx context.Context, p configs.Profile, action string, out, diagnostic io.Writer) int {
	if p.Auth.Method != "password" {
		return cliError(diagnostic, 2, errors.New("credential commands require password authentication"))
	}
	d, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		return cliError(diagnostic, 6, err)
	}
	defer d.Close()
	prompt := promptFor(p, diagnostic)
	store, err := credentials.New(p, prompt)
	if err != nil {
		return cliError(diagnostic, 6, err)
	}
	defer store.Close()
	status := ""
	switch action {
	case "status":
		status, err = store.Status(ctx)
	case "set":
		var password string
		password, err = prompt(ctx, "New VPN password:")
		if err == nil {
			err = store.Set(ctx, password)
		}
		status = "saved"
	case "delete":
		err = store.Delete(ctx)
		status = "deleted"
	}
	if err != nil {
		if errors.Is(err, terminal.ErrInteractionRequired) {
			return cliError(diagnostic, 3, err)
		}
		return cliError(diagnostic, 6, err)
	}
	_ = json.NewEncoder(out).Encode(map[string]string{"customer": p.ID, "credential": status})
	return 0
}

func loadMaterial(ctx context.Context, p configs.Profile, diagnostic io.Writer) (instance.Material, error) {
	var material instance.Material
	var err error
	if p.Auth.TOTPSecretFile != "" {
		material.TOTP, err = instance.ReadSecret(p.Auth.TOTPSecretFile)
		if err != nil {
			return material, err
		}
	}
	if p.Auth.CertificateFile != "" {
		data, err := securefile.ReadLimited(p.Auth.CertificateFile, 4<<20)
		if err != nil {
			return material, err
		}
		defer clear(data)
		var password string
		if p.Auth.CertificatePasswordFile != "" {
			password, err = instance.ReadSecret(p.Auth.CertificatePasswordFile)
		} else {
			password, err = promptFor(p, diagnostic)(ctx, "Client certificate password (empty if none):")
		}
		if err != nil {
			return material, err
		}
		key, cert, err := pkcs12.Decode(data, password)
		if err != nil {
			return material, errors.New("cannot decrypt client PKCS#12 certificate")
		}
		material.Certificate = tls.Certificate{Certificate: [][]byte{cert.Raw}, PrivateKey: key, Leaf: cert}
	}
	return material, nil
}

func authInfo(ctx context.Context, p configs.Profile, out, diagnostic io.Writer) int {
	if p.Protocol != "atrust" {
		_ = json.NewEncoder(out).Encode(map[string]any{"protocol": "easyconnect", "implemented_primary_methods": []string{"password", "certificate"}, "gateway_discovery": false})
		return 0
	}
	timeout, _ := time.ParseDuration(p.Session.SetupTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	base, err := underlay.New(underlay.Options{AutoDetect: false})
	if err != nil {
		return cliError(diagnostic, 5, err)
	}
	defer base.Close()
	policy, err := tlspolicy.New(tlspolicy.Options{Mode: p.TLS.Mode, CAFile: p.TLS.CAFile, ServerName: p.TLS.ServerName, Pins: p.TLS.Pins})
	if err != nil {
		return cliError(diagnostic, 2, err)
	}
	session := auth.NewSessionContext(ctx, p.Server, nil, base.DialContext)
	session.SetTLSConfig(policy.Standard())
	defer session.Close()
	info, err := session.GetAuthInfoList()
	if err != nil {
		return cliError(diagnostic, 5, err)
	}
	// Login links can contain transient tokens; auth-info lists only method metadata.
	safe := make([]map[string]string, 0, len(info))
	for _, i := range info {
		safe = append(safe, map[string]string{"domain": i.LoginDomain, "auth_type": i.AuthType, "name": i.AuthName, "sub_type": i.SubType})
	}
	_ = json.NewEncoder(out).Encode(safe)
	return 0
}
