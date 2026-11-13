package configs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mythologyli/zju-connect/internal/securefile"
	"go.yaml.in/yaml/v3"
)

const MaxConfigBytes = 64 << 10

// Profile is the public versioned schema. Secrets have file references only.
type Profile struct {
	SchemaVersion int         `yaml:"schema_version"`
	ID            string      `yaml:"id"`
	Protocol      string      `yaml:"protocol"`
	Server        string      `yaml:"server"`
	Auth          Auth        `yaml:"auth"`
	Credentials   Credentials `yaml:"credentials"`
	Listen        Listen      `yaml:"listen"`
	DNS           DNS         `yaml:"dns"`
	Session       Session     `yaml:"session"`
	TLS           TLS         `yaml:"tls"`
	EasyConnect   EasyConnect `yaml:"easyconnect"`
	ATrust        ATrust      `yaml:"atrust"`
	Debug         bool        `yaml:"debug"`
}

type Auth struct {
	Method                  string `yaml:"method"`
	Username                string `yaml:"username"`
	Phone                   string `yaml:"phone"`
	Domain                  string `yaml:"domain"`
	RememberPassword        bool   `yaml:"remember_password"`
	SecretFile              string `yaml:"secret_file"`
	TOTPSecretFile          string `yaml:"totp_secret_file"`
	CertificateFile         string `yaml:"certificate_file"`
	CertificatePasswordFile string `yaml:"certificate_password_file"`
	Interactive             string `yaml:"interactive"`
	Timeout                 string `yaml:"timeout"`
	CaptchaMode             string `yaml:"captcha_mode"`
	CaptchaFile             string `yaml:"captcha_file"`
	CallbackListen          string `yaml:"callback_listen"`
	CallbackURL             string `yaml:"callback_url"`
}

type Credentials struct {
	Backend   string `yaml:"backend"`
	Ref       string `yaml:"ref"`
	VaultFile string `yaml:"vault_file"`
}
type Listen struct {
	SOCKS5 string `yaml:"socks5"`
	HTTP   string `yaml:"http"`
}
type DNS struct {
	Mode    string   `yaml:"mode"`
	Servers []string `yaml:"servers"`
	TTL     int      `yaml:"ttl"`
}
type Session struct {
	StateDir          string `yaml:"state_dir"`
	SetupTimeout      string `yaml:"setup_timeout"`
	DialTimeout       string `yaml:"dial_timeout"`
	ReconnectAttempts int    `yaml:"reconnect_attempts"`
}
type TLS struct {
	Mode       string   `yaml:"mode"`
	CAFile     string   `yaml:"ca_file"`
	ServerName string   `yaml:"server_name"`
	Pins       []string `yaml:"pins"`
}
type EasyConnect struct {
	MultiLine      bool `yaml:"multi_line"`
	AllowLegacyTLS bool `yaml:"allow_legacy_tls"`
}
type ATrust struct {
	MACAddresses   []string `yaml:"mac_addresses"`
	SessionRefresh string   `yaml:"session_refresh"`
	NodesRefresh   string   `yaml:"nodes_refresh"`
}

func profileDefaults() Profile {
	return Profile{
		Auth:        Auth{Method: "password", Interactive: "auto", Timeout: "5m", CaptchaMode: "auto", CallbackListen: "127.0.0.1:0"},
		Credentials: Credentials{Backend: "auto"},
		Listen:      Listen{SOCKS5: "127.0.0.1:1080", HTTP: "127.0.0.1:1081"},
		DNS:         DNS{Mode: "vpn", TTL: 300},
		Session:     Session{SetupTimeout: "10m", DialTimeout: "15s", ReconnectAttempts: 3},
		TLS:         TLS{Mode: "verify"},
		ATrust:      ATrust{SessionRefresh: "30m", NodesRefresh: "5m"},
	}
}

func LoadProfile(path string) (Profile, error) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".yaml" && extension != ".yml" {
		return Profile{}, errors.New("configuration must use .yaml or .yml")
	}
	data, err := securefile.ReadLimited(path, MaxConfigBytes)
	if err != nil {
		return Profile{}, fmt.Errorf("read configuration: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Profile{}, err
	}
	return ParseProfile(data, filepath.Dir(absolute))
}

func ParseProfile(data []byte, baseDir string) (Profile, error) {
	if len(data) == 0 || len(data) > MaxConfigBytes {
		return Profile{}, errors.New("configuration is empty or exceeds 64 KiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return Profile{}, errors.New("invalid YAML syntax")
	}
	if len(document.Content) != 1 {
		return Profile{}, errors.New("configuration must contain one mapping")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return Profile{}, errors.New("configuration must contain exactly one YAML document")
	}
	count := 0
	if err := validateNode(document.Content[0], reflect.TypeOf(Profile{}), 0, &count); err != nil {
		return Profile{}, err
	}
	profile := profileDefaults()
	if err := document.Decode(&profile); err != nil {
		return Profile{}, errors.New("invalid configuration field type")
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	if profile.Session.StateDir == "" {
		profile.Session.StateDir = filepath.Join("state", profile.ID)
	}
	if profile.Credentials.Ref == "" {
		profile.Credentials.Ref = profile.ID
	}
	for _, path := range []*string{
		&profile.Session.StateDir, &profile.Auth.SecretFile, &profile.Auth.TOTPSecretFile,
		&profile.Auth.CertificateFile, &profile.Auth.CertificatePasswordFile,
		&profile.Auth.CaptchaFile, &profile.Credentials.VaultFile, &profile.TLS.CAFile,
	} {
		if *path != "" && !filepath.IsAbs(*path) {
			*path = filepath.Clean(filepath.Join(baseDir, *path))
		}
	}
	return profile, nil
}

// Validate the syntax tree before decoding: yaml's normal string coercions and
// alias expansion are intentionally not part of this configuration language.
func validateNode(node *yaml.Node, schema reflect.Type, depth int, count *int) error {
	*count++
	invalid := func(message string) error { return fmt.Errorf("configuration line %d: %s", node.Line, message) }
	if depth > 16 || *count > 2048 {
		return invalid("configuration nesting or node limit exceeded")
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return invalid("YAML anchors and aliases are not supported")
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return invalid("explicit YAML tags are not supported")
	}
	switch schema.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return invalid("expected a mapping")
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < schema.NumField(); i++ {
			field := schema.Field(i)
			fields[field.Tag.Get("yaml")] = field.Type
		}
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Anchor != "" || key.Style&yaml.TaggedStyle != 0 {
				return invalid("YAML anchors and explicit tags are not supported")
			}
			field, known := fields[key.Value]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !known {
				return fmt.Errorf("configuration line %d: unknown configuration field", key.Line)
			}
			if seen[key.Value] {
				return fmt.Errorf("configuration line %d: duplicate configuration field", key.Line)
			}
			seen[key.Value] = true
			if err := validateNode(value, field, depth+1, count); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return invalid("expected a sequence")
		}
		for _, child := range node.Content {
			if err := validateNode(child, schema.Elem(), depth+1, count); err != nil {
				return err
			}
		}
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return invalid("expected a string")
		}
	case reflect.Bool:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
			return invalid("expected a boolean")
		}
	case reflect.Int:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return invalid("expected an integer")
		}
	default:
		return invalid("unsupported configuration type")
	}
	return nil
}

var profileID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func (p *Profile) Validate() error {
	if p.SchemaVersion != 1 {
		return errors.New("schema_version must be 1")
	}
	if !profileID.MatchString(p.ID) {
		return errors.New("id must be 1–64 ASCII letters, digits, dots, hyphens or underscores, starting with a letter or digit")
	}
	if p.Protocol != "easyconnect" && p.Protocol != "atrust" {
		return errors.New("protocol must be easyconnect or atrust")
	}
	server, err := NormalizeServer(p.Server)
	if err != nil {
		return err
	}
	p.Server = server
	if p.Listen.HTTP == "" && p.Listen.SOCKS5 == "" {
		return errors.New("at least one proxy listener is required")
	}
	for _, address := range []string{p.Listen.HTTP, p.Listen.SOCKS5} {
		if address != "" {
			if err := validateListen(address, false); err != nil {
				return err
			}
		}
	}
	if p.Listen.HTTP != "" && p.Listen.HTTP == p.Listen.SOCKS5 {
		return errors.New("HTTP and SOCKS5 must use different listeners")
	}
	if p.DNS.Mode != "vpn" {
		return errors.New("dns.mode must be vpn")
	}
	if len(p.DNS.Servers) > 2 {
		return errors.New("dns.servers supports at most two VPN resolvers")
	}
	for _, server := range p.DNS.Servers {
		if ip := net.ParseIP(server); ip == nil || ip.To4() == nil {
			return errors.New("dns.servers must contain IPv4 resolver addresses")
		}
	}
	if p.DNS.TTL < 1 || p.DNS.TTL > 86400 {
		return errors.New("dns.ttl must be between 1 and 86400 seconds")
	}
	switch p.Auth.Method {
	case "password", "sms", "cas", "oauth2", "certificate":
	default:
		return errors.New("unsupported auth.method")
	}
	if p.Protocol == "easyconnect" && p.Auth.Method != "password" && p.Auth.Method != "certificate" {
		return errors.New("EasyConnect supports password or certificate primary authentication")
	}
	if p.Protocol == "atrust" && p.Auth.Method == "certificate" {
		return errors.New("aTrust certificate primary authentication is not implemented")
	}
	if p.Auth.RememberPassword && p.Auth.Method != "password" {
		return errors.New("remember_password applies only to password authentication")
	}
	if p.Auth.Interactive != "auto" && p.Auth.Interactive != "never" {
		return errors.New("auth.interactive must be auto or never")
	}
	switch p.Auth.CaptchaMode {
	case "auto", "ansi", "file":
	default:
		return errors.New("auth.captcha_mode must be auto, ansi or file")
	}
	if p.Auth.CaptchaMode == "file" && p.Auth.CaptchaFile == "" {
		return errors.New("auth.captcha_file is required for file mode")
	}
	if err := validateListen(p.Auth.CallbackListen, true); err != nil {
		return fmt.Errorf("auth.callback_listen: %w", err)
	}
	callbackHost, callbackPort, _ := net.SplitHostPort(p.Auth.CallbackListen)
	if !net.ParseIP(callbackHost).IsLoopback() && (p.Auth.CallbackURL == "" || callbackPort == "0") {
		return errors.New("non-loopback callback_listen requires a fixed port and callback_url for loopback host publishing")
	}
	if p.Auth.CallbackURL != "" {
		u, err := url.Parse(p.Auth.CallbackURL)
		if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("auth.callback_url must be an HTTP loopback origin")
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() || validateListen(u.Host, false) != nil {
			return errors.New("auth.callback_url requires a loopback IP and port")
		}
	}
	if p.Credentials.Backend != "auto" && p.Credentials.Backend != "age-file" {
		return errors.New("credentials.backend must be auto or age-file")
	}
	if p.Credentials.Backend == "age-file" && p.Credentials.VaultFile == "" {
		return errors.New("credentials.vault_file is required for age-file")
	}
	if len(p.Credentials.Ref) > 128 {
		return errors.New("credentials.ref is too long")
	}
	switch p.TLS.Mode {
	case "verify", "pin", "insecure":
	default:
		return errors.New("tls.mode must be verify, pin or insecure")
	}
	if p.TLS.Mode == "pin" && len(p.TLS.Pins) == 0 {
		return errors.New("tls.pins is required for pin mode")
	}
	if p.TLS.Mode != "pin" && len(p.TLS.Pins) != 0 {
		return errors.New("tls.pins requires pin mode")
	}
	if len(p.TLS.Pins) > 8 {
		return errors.New("too many TLS pins")
	}
	if p.Session.ReconnectAttempts < 0 || p.Session.ReconnectAttempts > 10 {
		return errors.New("session.reconnect_attempts must be between 0 and 10")
	}
	for name, value := range map[string]string{
		"auth.timeout": p.Auth.Timeout, "session.setup_timeout": p.Session.SetupTimeout,
		"session.dial_timeout": p.Session.DialTimeout, "atrust.session_refresh": p.ATrust.SessionRefresh,
		"atrust.nodes_refresh": p.ATrust.NodesRefresh,
	} {
		duration, err := time.ParseDuration(value)
		if err != nil || duration < time.Second || duration > 24*time.Hour {
			return fmt.Errorf("%s must be a duration between 1s and 24h", name)
		}
	}
	for _, mac := range p.ATrust.MACAddresses {
		if _, err := net.ParseMAC(mac); err != nil {
			return errors.New("invalid atrust.mac_addresses entry")
		}
	}
	return nil
}

func (p Profile) ValidateForRun() error {
	if p.Auth.Method == "password" && p.Auth.Username == "" {
		return errors.New("auth.username is required for password authentication")
	}
	if p.Auth.Method == "sms" && p.Auth.Phone == "" {
		return errors.New("auth.phone is required for SMS primary authentication")
	}
	if p.Auth.Method == "certificate" && p.Auth.CertificateFile == "" {
		return errors.New("auth.certificate_file is required")
	}
	return nil
}

func NormalizeServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" || strings.ContainsAny(server, "/\\@?#\r\n\t ") {
		return "", errors.New("server must be a host or host:port without URL, userinfo or path")
	}
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		if strings.Contains(server, ":") && net.ParseIP(server) == nil {
			return "", errors.New("invalid server address")
		}
		host, port = server, "443"
	}
	if host == "" {
		return "", errors.New("server host is empty")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", errors.New("invalid server port")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		host = strings.TrimSuffix(strings.ToLower(host), ".")
		if len(host) > 253 {
			return "", errors.New("server hostname is too long")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", errors.New("invalid server hostname")
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
					return "", errors.New("server hostname must use ASCII DNS labels")
				}
			}
		}
	}
	if host == "" {
		return "", errors.New("server host is empty")
	}
	return net.JoinHostPort(host, strconv.Itoa(number)), nil
}

func validateListen(address string, allowZero bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil {
		return errors.New("listener must use an explicit IP address and port")
	}
	number, err := strconv.Atoi(port)
	minimum := 1
	if allowZero {
		minimum = 0
	}
	if err != nil || number < minimum || number > 65535 {
		return errors.New("invalid listener port")
	}
	return nil
}
