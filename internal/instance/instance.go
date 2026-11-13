// Package instance owns one customer's connection, resolver and listeners.
package instance

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/client/atrust"
	"github.com/mythologyli/zju-connect/client/atrust/auth"
	"github.com/mythologyli/zju-connect/client/authchallenge"
	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/internal/credentials"
	"github.com/mythologyli/zju-connect/internal/identity"
	"github.com/mythologyli/zju-connect/internal/state"
	"github.com/mythologyli/zju-connect/internal/terminal"
	"github.com/mythologyli/zju-connect/internal/tlspolicy"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/stack"
	"github.com/mythologyli/zju-connect/stack/gvisor"
	"github.com/mythologyli/zju-connect/stack/tcptunnel"
	"github.com/mythologyli/zju-connect/underlay"
)

type Status struct {
	SchemaVersion int       `json:"schema_version"`
	Customer      string    `json:"customer"`
	Generation    uint64    `json:"generation"`
	State         string    `json:"state"`
	Reason        string    `json:"reason,omitempty"`
	PID           int       `json:"pid"`
	At            time.Time `json:"at"`
}

type Frontend interface {
	Password(context.Context) (string, error)
	Challenge(context.Context, uint64, func(string), func()) authchallenge.Handler
	Authenticated(context.Context)
	Status(Status)
}
type Material struct {
	TOTP        string
	Certificate tls.Certificate
}
type Failure struct {
	Code int
	Err  error
}

func (f *Failure) Error() string { return f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type owner struct {
	client   client.Client
	stack    stack.Stack
	resolver *resolve.Resolver
	proxies  *serviceListeners
	underlay *underlay.Dialer
}

// Run never resubmits a password during recovery: reconnection uses only the
// saved session. An invalid session returns to the foreground authentication flow.
func Run(ctx context.Context, p configs.Profile, material Material, ui Frontend) error {
	if err := p.ValidateForRun(); err != nil {
		return &Failure{2, err}
	}
	policy, err := tlspolicy.New(tlspolicy.Options{Mode: p.TLS.Mode, CAFile: p.TLS.CAFile, ServerName: p.TLS.ServerName, Pins: p.TLS.Pins})
	if err != nil {
		return &Failure{2, err}
	}
	directory, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		return &Failure{6, err}
	}
	defer directory.Close()
	binding := identity.ForProfile(p)
	var generation uint64 = 1
	emit := func(status, reason string) {
		value := Status{SchemaVersion: 1, Customer: p.ID, Generation: generation, State: status, Reason: reason, PID: os.Getpid(), At: time.Now().UTC()}
		ui.Status(value)
		if err := directory.SaveStatus(value); err != nil {
			log.Println("Unable to persist instance status")
		}
	}
	emit("idle", "")
	everReady := false
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			emit("stopping", "")
			return nil
		}
		saved, err := directory.LoadSession(binding)
		if err != nil {
			emit("failed", "state_error")
			return &Failure{6, err}
		}
		if attempt > 0 {
			emit("reconnecting", "transport_failed")
			delay := time.Second * time.Duration(1<<min(attempt-1, 5))
			timer := time.NewTimer(delay + time.Duration(rand.Int64N(int64(delay/4)+1)))
			select {
			case <-ctx.Done():
				timer.Stop()
				emit("stopping", "")
				return nil
			case <-timer.C:
			}
			generation++
		}
		connection, err := reserve(p)
		if err != nil {
			emit("failed", "listener_error")
			return &Failure{2, err}
		}
		emit("connecting", "")
		setupCtx, cancel := context.WithTimeout(ctx, duration(p.Session.SetupTimeout))
		handler := ui.Challenge(setupCtx, generation, func(kind string) { emit("waiting_auth", kind) }, func() { emit("authenticating", "") })
		emit("authenticating", "")
		save := func(data []byte) error { return directory.SaveSession(binding, data) }
		err = connection.setup(setupCtx, p, material, policy, saved, everReady, ui.Password, handler, save)
		cancel()
		if err == nil {
			err = connection.start(ctx, p)
			if err == nil {
				everReady = true
				ui.Authenticated(ctx)
				emit("ready", "")
				err = connection.wait(ctx)
			}
		}
		err = connection.failureCause(err)
		connection.close()
		if ctx.Err() != nil || errors.Is(err, terminal.ErrCanceled) {
			emit("stopping", "")
			return nil
		}
		if errors.Is(err, terminal.ErrInteractionRequired) || errors.Is(err, auth.ErrSessionInvalid) || errors.Is(err, easyconnect.ErrSangforShutdown) {
			if !errors.Is(err, terminal.ErrInteractionRequired) {
				_ = directory.ClearSession()
			}
			emit("waiting_auth", "authentication_required")
			return &Failure{3, err}
		}
		if !everReady || !retryable(err) || attempt >= p.Session.ReconnectAttempts {
			code := 5
			if errors.Is(err, client.ErrAuthenticationRejected) {
				code = 4
			}
			if errors.Is(err, credentials.ErrUnavailable) || errors.Is(err, credentials.ErrLocked) || errors.Is(err, credentials.ErrBinding) || errors.Is(err, credentials.ErrDecrypt) {
				code = 6
			}
			emit("failed", "connection_failed")
			return &Failure{code, err}
		}
	}
}

func duration(value string) time.Duration { duration, _ := time.ParseDuration(value); return duration }
func retryable(err error) bool {
	if err == nil || errors.Is(err, client.ErrAuthenticationRejected) {
		return false
	}
	var verification *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	return !errors.As(err, &verification) && !errors.As(err, &unknown) && !errors.As(err, &hostname)
}

func (o *owner) setup(ctx context.Context, p configs.Profile, material Material, policy *tlspolicy.Policy, saved []byte, resumeOnly bool, password func(context.Context) (string, error), handler authchallenge.Handler, save func([]byte) error) error {
	var err error
	o.underlay, err = underlay.New(underlay.Options{AutoDetect: false})
	if err != nil {
		return err
	}
	if p.Protocol == "easyconnect" {
		var snapshot struct {
			Server    string `json:"server"`
			SessionID string `json:"session_id"`
		}
		if len(saved) > 0 && json.Unmarshal(saved, &snapshot) != nil {
			return errors.New("invalid EasyConnect saved session")
		}
		server := p.Server
		if snapshot.Server != "" {
			server, err = configs.NormalizeServer(snapshot.Server)
			if err != nil {
				return err
			}
		}
		if resumeOnly && snapshot.SessionID == "" {
			return terminal.ErrInteractionRequired
		}
		var provider func(context.Context) (string, error)
		if p.Auth.Method == "password" {
			provider = password
		}
		ec := easyconnect.NewClient(easyconnect.Options{Server: server, TLSPolicy: policy, AllowLegacyTLS: p.EasyConnect.AllowLegacyTLS, UnderlayDialer: o.underlay, SessionID: snapshot.SessionID, TestMultiLine: p.EasyConnect.MultiLine && !resumeOnly, Auth: easyconnect.AuthOptions{Username: p.Auth.Username, PasswordProvider: provider, TOTPSecret: material.TOTP, Certificate: material.Certificate}, Resources: easyconnect.ResourceOptions{Fetch: true, IncludeDomains: true}, ChallengeHandler: handler})
		o.client = ec
		if err := ec.SetupContext(ctx); err != nil {
			return err
		}
		snapshot.Server, snapshot.SessionID = ec.SessionSnapshot()
		data, _ := json.Marshal(snapshot)
		if err := save(data); err != nil {
			log.Println("Authenticated, but session state was not saved")
		}
	} else {
		at := atrust.NewClient(atrust.ClientOptions{TLSPolicy: policy, UnderlayDialer: o.underlay, Session: atrust.SessionOptions{Username: p.Auth.Username, MACAddresses: p.ATrust.MACAddresses}})
		o.client = at
		var method auth.LoginMethod
		if !resumeOnly {
			switch p.Auth.Method {
			case "password":
				method = auth.PasswordLogin{Username: p.Auth.Username, Domain: p.Auth.Domain, PasswordProvider: password}
			case "sms":
				method = auth.SMSLogin{Phone: p.Auth.Phone, Domain: p.Auth.Domain}
			case "cas":
				method = auth.CASLogin{Domain: p.Auth.Domain}
			case "oauth2":
				method = auth.HTTPSOauth2Login{Domain: p.Auth.Domain}
			}
		}
		host, port, _ := net.SplitHostPort(p.Server)
		number, _ := strconv.Atoi(port)
		_, err := at.SetupContext(ctx, atrust.SetupOptions{ServerAddress: host, ServerPort: number, LoginMethod: method, TOTPSecret: material.TOTP, ClientData: saved, ChallengeHandler: handler, BestNodesRefreshInterval: duration(p.ATrust.NodesRefresh), SessionRefreshInterval: duration(p.ATrust.SessionRefresh), SaveClientData: func(data []byte) error {
			if err := save(data); err != nil {
				log.Println("Authenticated, but session state was not saved")
			}
			return nil
		}})
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *owner) start(ctx context.Context, p configs.Profile) error {
	var err error
	if p.Protocol == "easyconnect" && o.client.CanUseTCPTunnel() {
		o.stack, err = tcptunnel.NewStack(o.client)
	} else {
		o.stack, err = gvisor.NewStack(o.client)
	}
	if err != nil {
		return err
	}
	ipResources, _ := o.client.IPResources()
	domainResources, _ := o.client.DomainResources()
	dnsResources, _ := o.client.DNSResource()
	servers := p.DNS.Servers
	if len(servers) == 0 {
		servers, _ = o.client.DNSServers()
	}
	var primary, secondary string
	for _, server := range servers {
		if ip := net.ParseIP(server); ip != nil && ip.To4() != nil {
			if primary == "" {
				primary = ip.String()
			} else if secondary == "" {
				secondary = ip.String()
			}
		}
	}
	o.resolver = resolve.NewResolver(o.stack, primary, secondary, uint64(p.DNS.TTL), domainResources, dnsResources, primary != "")
	o.stack.SetupIPPool(o.resolver.IPPool)
	if starter, ok := o.stack.(interface{ Start(context.Context) error }); ok {
		startCtx, cancel := context.WithTimeout(ctx, duration(p.Session.DialTimeout))
		defer cancel()
		if err := starter.Start(startCtx); err != nil {
			return err
		}
	}
	dialer := dial.NewDialer(o.stack, o.resolver, ipResources)
	o.proxies.Start(dialer, o.resolver, duration(p.Session.DialTimeout))
	return nil
}

func (o *owner) wait(ctx context.Context) error {
	var clientErrors, stackErrors <-chan error
	if source, ok := o.client.(interface{ Errors() <-chan error }); ok {
		clientErrors = source.Errors()
	}
	if source, ok := o.stack.(interface{ Errors() <-chan error }); ok {
		stackErrors = source.Errors()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-o.proxies.Errors():
		return err
	case err := <-clientErrors:
		return err
	case err := <-stackErrors:
		return err
	}
}

func (o *owner) failureCause(fallback error) error {
	if source, ok := o.client.(interface{ Errors() <-chan error }); ok {
		select {
		case err := <-source.Errors():
			return err
		default:
		}
	}
	return fallback
}
func (o *owner) close() {
	if o.proxies != nil {
		o.proxies.Close()
	}
	if o.resolver != nil {
		o.resolver.Close()
	}
	if closer, ok := o.client.(interface{ Close() }); ok {
		closer.Close()
	}
	if closer, ok := o.stack.(interface{ Close() error }); ok {
		closer.Close()
	}
	if o.underlay != nil {
		o.underlay.Close()
	}
}
