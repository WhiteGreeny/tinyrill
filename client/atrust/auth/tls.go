package auth

import (
	"crypto/tls"
	"net/http"

	"github.com/mythologyli/zju-connect/internal/authdiag"
)

func (s *Session) Close() { s.client.CloseIdleConnections() }

// SetDiagnostics must be called before publishing or using the session.
func (s *Session) SetDiagnostics(observer authdiag.Observer) { s.diagnostics = observer }

// SetTLSConfig applies caller-selected trust before authentication starts.
func (s *Session) SetTLSConfig(config *tls.Config) {
	transport := s.client.Transport.(*http.Transport)
	replacement := transport.Clone()
	replacement.TLSClientConfig = config.Clone()
	if replacement.TLSClientConfig.KeyLogWriter == nil {
		replacement.TLSClientConfig.KeyLogWriter = transport.TLSClientConfig.KeyLogWriter
	}
	transport.CloseIdleConnections()
	s.client.Transport = replacement
}
