package atrust

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestMalformedNodeAddressesNeverPanic(t *testing.T) {
	result := getBestReachableNodes(context.Background(), map[string][]string{"g": {"invalid", "host:bad", ":443", "host:0", "host:65536", "[bad"}}, nil, nil)
	if len(result) != 0 {
		t.Fatal("invalid address selected")
	}
}

func TestRuntimeTunnelFailureIsOwnedByInstance(t *testing.T) {
	c := NewClient(ClientOptions{})
	defer c.Close()
	c.setIP(net.IPv4(198, 18, 0, 2))
	tunnel, err := NewL3Tunnel(c)
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.Close()
	tunnel.startReconnect("group")
	select {
	case err := <-c.Errors():
		if err == nil {
			t.Fatal("nil failure")
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not report disconnect to its owner")
	}
	if c.lifecycleCtx.Err() == nil {
		t.Fatal("old session remained available")
	}
}
func FuzzResourceParser(f *testing.F) {
	f.Add([]byte(`{"data":{"appList":{"data":{}}}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		c := &Client{}
		_ = c.parseResource(b)
	})
}
