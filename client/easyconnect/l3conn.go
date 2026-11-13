package easyconnect

import (
	"io"
	"sync"

	"github.com/mythologyli/zju-connect/log"
)

type L3Conn struct {
	easyConnectClient *Client

	sendConn io.WriteCloser
	sendLock sync.Mutex

	recvConn io.ReadCloser
	recvLock sync.Mutex
}

// The session owner recreates a failed tunnel and closes old business streams.
func (c *L3Conn) Read(p []byte) (n int, err error) {
	c.recvLock.Lock()
	defer c.recvLock.Unlock()
	return c.recvConn.Read(p)
}

func (c *L3Conn) Write(p []byte) (n int, err error) {
	c.sendLock.Lock()
	defer c.sendLock.Unlock()
	return c.sendConn.Write(p)
}

func (c *L3Conn) Close() error {
	if c.sendConn != nil {
		_ = c.sendConn.Close()
	}
	if c.recvConn != nil {
		_ = c.recvConn.Close()
	}
	return nil
}

func (c *Client) NewL3Conn() (io.ReadWriteCloser, error) {
	conn := &L3Conn{
		easyConnectClient: c,
	}

	var err error
	conn.sendConn, err = c.SendConn()
	if err != nil {
		log.Printf("Error occurred while creating sendConn: %v", err)
		return nil, err
	}

	conn.recvConn, err = c.RecvConn()
	if err != nil {
		_ = conn.sendConn.Close()
		log.Printf("Error occurred while creating recvConn: %v", err)
		return nil, err
	}
	return conn, nil
}
