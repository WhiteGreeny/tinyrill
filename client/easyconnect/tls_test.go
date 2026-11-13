package easyconnect

import (
	"context"
	"net/http"
	"testing"
)

func TestEveryEasyConnectChannelVerifiesIdentityByDefault(t *testing.T) {
	c := NewClient(Options{Server: "vpn.example:443"})
	defer c.Close()
	if c.httpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify {
		t.Fatal("authentication disables certificate verification")
	}
	config := c.uTLSConfig()
	if config.InsecureSkipVerify || config.ServerName != "vpn.example" {
		t.Fatal("raw TLS channel lacks certificate/hostname verification")
	}
	if _, err := c.tlsConn(context.Background(), "TCPP"); err == nil {
		t.Fatal("legacy cipher enabled without explicit consent")
	}
}
