package auth

import (
	"crypto/tls"
	"net/http"
)

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
