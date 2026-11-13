// Package tlspolicy applies an explicit customer trust policy to every channel.
package tlspolicy

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net"
	"strings"

	"github.com/mythologyli/zju-connect/internal/securefile"
)

type Options struct {
	Mode, CAFile, ServerName string
	Pins                     []string
}
type Policy struct {
	mode, serverName string
	roots            *x509.CertPool
	pins             [][sha256.Size]byte
}

func New(options Options) (*Policy, error) {
	p := &Policy{mode: options.Mode, serverName: options.ServerName}
	if p.mode == "" {
		p.mode = "verify"
	}
	if p.mode != "verify" && p.mode != "pin" && p.mode != "insecure" {
		return nil, errors.New("unsupported TLS policy")
	}
	if options.CAFile != "" {
		data, err := securefile.ReadLimited(options.CAFile, 1<<20)
		if err != nil {
			return nil, err
		}
		p.roots = x509.NewCertPool()
		if !p.roots.AppendCertsFromPEM(data) {
			return nil, errors.New("TLS CA file contains no certificates")
		}
	}
	for _, pin := range options.Pins {
		encoded, ok := strings.CutPrefix(pin, "sha256/")
		if !ok {
			return nil, errors.New("TLS pins must use sha256/ followed by a base64 SPKI digest")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) != sha256.Size {
			return nil, errors.New("invalid TLS SPKI pin")
		}
		var digest [sha256.Size]byte
		copy(digest[:], data)
		p.pins = append(p.pins, digest)
	}
	if p.mode == "pin" && len(p.pins) == 0 {
		return nil, errors.New("pin mode requires a trusted TLS SPKI pin")
	}
	if p.mode != "pin" && len(p.pins) != 0 {
		return nil, errors.New("TLS pins require pin mode")
	}
	return p, nil
}

// Standard defaults to normal certificate-chain and hostname verification.
// A nil policy is the safe default, including for directly constructed clients.
func (p *Policy) Standard() *tls.Config {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if p == nil {
		return config
	}
	config.RootCAs, config.ServerName = p.roots, p.serverName
	switch p.mode {
	case "insecure":
		config.InsecureSkipVerify = true // explicitly selected customer exception
	case "pin":
		config.InsecureSkipVerify = true // replaced by the independently supplied SPKI identity
		config.VerifyPeerCertificate = p.verifyPin
	}
	return config
}

func (p *Policy) ForAddress(address string) *tls.Config {
	config := p.Standard()
	if config.ServerName == "" {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		config.ServerName = host
	}
	return config
}

func (p *Policy) verifyPin(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errors.New("TLS peer did not supply a certificate")
	}
	certificate, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return errors.New("TLS peer certificate is invalid")
	}
	digest := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	for _, pin := range p.pins {
		if subtle.ConstantTimeCompare(digest[:], pin[:]) == 1 {
			return nil
		}
	}
	return errors.New("TLS peer public key does not match a trusted pin")
}
