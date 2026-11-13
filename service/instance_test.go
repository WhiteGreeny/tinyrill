package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/resolve"
)

func TestReserveRollsBackPortsOnConflict(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if s, err := Reserve("127.0.0.1:0", busy.Addr().String()); err == nil {
		s.Close()
		t.Fatal("port conflict accepted")
	}
}

func TestCloseStopsSOCKSHandshakeAndOtherInstanceSurvives(t *testing.T) {
	instances := make([]*Listeners, 3)
	connections := make([]net.Conn, 3)
	for i := range instances {
		s, err := Reserve("127.0.0.1:0", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		r := resolve.NewResolver(nil, "", "", 60, nil, nil, false)
		defer r.Close()
		s.Start(dial.NewDialer(nil, r, nil), r, time.Second)
		instances[i] = s
		conn, err := net.Dial("tcp", s.socks.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		connections[i] = conn
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		conn.Write([]byte{5, 1, 0})
		var handshake [2]byte
		if _, err := io.ReadFull(conn, handshake[:]); err != nil {
			t.Fatal(err)
		}
	}
	instances[0].Close()
	var b [1]byte
	if _, err := connections[0].Read(b[:]); err == nil {
		t.Fatal("closed instance left accepted stream open")
	}
	for i := 1; i < 3; i++ {
		// An unsupported UDP request is rejected by each still-running instance.
		connections[i].Write([]byte{5, 3, 0, 1, 10, 0, 0, 1, 0, 53})
		var reply [10]byte
		if _, err := io.ReadFull(connections[i], reply[:]); err != nil {
			t.Fatal(err)
		}
		if reply[1] == 0 {
			t.Fatal("UDP ASSOCIATE accepted")
		}
	}
}

func TestCloseStopsHTTPHijackedStream(t *testing.T) {
	s, err := Reserve("127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	upstream, peer := net.Pipe()
	defer peer.Close()
	s.proxy = newHTTPProxyWithDial(s.wrapDial(func(context.Context, string, string) (net.Conn, error) { return upstream, nil }, time.Second))
	s.server = newHTTPServer(s.http.Addr().String(), s.proxy)
	s.serve(func() error { return s.server.Serve(&trackingListener{Listener: s.http, pool: &s.pool}) })
	conn, err := net.Dial("tcp", s.http.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprint(conn, "CONNECT internal.example:443 HTTP/1.1\r\nHost: internal.example:443\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("CONNECT: %v", err)
	}
	s.Close()
	var b [1]byte
	if _, err := conn.Read(b[:]); err == nil {
		t.Fatal("hijacked stream remained open")
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := peer.Read(b[:]); err == nil {
		t.Fatal("upstream remained open")
	}
}
