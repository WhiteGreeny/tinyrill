package gvisor

import (
	"context"
	"github.com/mythologyli/zju-connect/client"
	"io"
	"net"
	"testing"
	"time"
)

type lifecycleClient struct {
	client.Client
	conn io.ReadWriteCloser
}

func (*lifecycleClient) IP() (net.IP, error)                      { return net.ParseIP("10.0.0.2"), nil }
func (c *lifecycleClient) NewL3Conn() (io.ReadWriteCloser, error) { return c.conn, nil }

func TestStackCloseUnblocksReadAndIsIdempotent(t *testing.T) {
	local, peer := net.Pipe()
	defer peer.Close()
	s, err := NewStack(&lifecycleClient{conn: local})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.Close(); s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close left the packet reader blocked")
	}
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("closed stack restarted")
	}
}

func TestStackReportsTunnelFailure(t *testing.T) {
	local, peer := net.Pipe()
	s, err := NewStack(&lifecycleClient{conn: local})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	peer.Close()
	select {
	case err := <-s.Errors():
		if err == nil {
			t.Fatal("missing error")
		}
	case <-time.After(time.Second):
		t.Fatal("tunnel failure was not reported")
	}
}
