package easyconnect

import utls "github.com/refraction-networking/utls"

func (c *Client) uTLSConfig() *utls.Config {
	standard := c.tlsPolicy.ForAddress(c.server)
	return &utls.Config{
		ServerName: standard.ServerName, RootCAs: standard.RootCAs,
		InsecureSkipVerify:    standard.InsecureSkipVerify,
		VerifyPeerCertificate: standard.VerifyPeerCertificate,
		MinVersion:            standard.MinVersion, KeyLogWriter: c.tlsKeyLogWriter,
	}
}
