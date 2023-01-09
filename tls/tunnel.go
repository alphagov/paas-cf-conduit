package tls

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/alphagov/paas-cf-conduit/logging"
	"github.com/alphagov/paas-cf-conduit/util"
)

type Tunnel struct {
	localAddr      string
	remoteAddr     string
	actualAddr     string
	listener       net.Listener
	errorChan      chan error
	tlsCipherSuite []uint16
	tlsMinVersion  uint16
	insecure       bool
}

func NewTunnel(localAddr, remoteAddr, actualAddr string, insecure bool, tlsCipherSuite []uint16, tlsMinVersion uint16) *Tunnel {
	return &Tunnel{
		localAddr:      localAddr,
		remoteAddr:     remoteAddr,
		actualAddr:     actualAddr,
		errorChan:      make(chan error, 8),
		tlsCipherSuite: tlsCipherSuite,
		tlsMinVersion:  tlsMinVersion,
		insecure:       insecure,
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
	var tlsConfig *tls.Config

	// This horrible hack is to work around an certificate validation issue on OXS
	// See the comment in util.GetRootCAs for more details
	// Hopefully this can be removed once we have a better solution

	_, err := os.Stat("/etc/ssl/cert.pem")
	if runtime.GOOS == "darwin" && err == nil && !t.insecure {
		rootCAs, err := util.GetRootCAs(nil)
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{
			ServerName:   strings.Split(t.actualAddr, ":")[0],
			RootCAs:      rootCAs,
			CipherSuites: t.tlsCipherSuite,
			MinVersion:   t.tlsMinVersion,
		}
	} else {
		tlsConfig = &tls.Config{
			ServerName:         strings.Split(t.actualAddr, ":")[0],
			InsecureSkipVerify: t.insecure,
			CipherSuites:       t.tlsCipherSuite,
			MinVersion:         t.tlsMinVersion,
		}
	}

	rconn, err := tls.Dial("tcp", t.remoteAddr, tlsConfig)
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
