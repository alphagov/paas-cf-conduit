package tls

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/alphagov/paas-cf-conduit/logging"
)

type Tunnel struct {
	localAddr  string
	remoteAddr string
	listener   net.Listener
	errorChan  chan error
}

func NewTunnel(localAddr, remoteAddr string) *Tunnel {
	return &Tunnel{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
		errorChan:  make(chan error, 8),
	}
}

func (t *Tunnel) Start() (chan error, error) {
	logging.Debug("starting TLS tunnel at", t.localAddr, "to", t.remoteAddr)
	var err error
	t.listener, err = net.Listen("tcp", t.localAddr)
	if err != nil {
		return nil, fmt.Errorf("starting a TLS tunnel failed: %s", err.Error())
	}
	go t.run()
	return t.errorChan, nil
}

func (t *Tunnel) Stop() error {
	return t.listener.Close()
}

func (t *Tunnel) run() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			t.errorChan <- fmt.Errorf("error accepting TLS connection: %s", err)
			continue
		}
		logging.Debug("accepted TLS connection to", t.localAddr, "from", conn.RemoteAddr())
		go func() {
			err := t.handleRequest(conn)
			if err != nil {
				t.errorChan <- err
			}
		}()
	}
}

func (t *Tunnel) handleRequest(conn net.Conn) error {
	tlsConfig := tls.Config{
		InsecureSkipVerify: true,
	}
	rconn, err := tls.Dial("tcp", t.remoteAddr, &tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %s", t.remoteAddr, "err")
	}

	go t.forward(conn, rconn)
	go t.forward(rconn, conn)

	return nil
}

func (t *Tunnel) forward(dst, src net.Conn) {
	_, err := io.Copy(dst, src)
	if err != nil && err != io.EOF {
		t.errorChan <- fmt.Errorf("failed to send data: %s", err)
	}
}
