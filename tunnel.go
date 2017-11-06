package main

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

type ForwardAddrs struct {
	LocalAddr   string
	RemoteAddr  string
	Credentials *Credentials
}

type Tunnel struct {
	TunnelAddr   string
	ForwardAddrs []ForwardAddrs
	AppGuid      string
	PasswordFunc func() (string, error)
	shutdownChan chan struct{}
	shutdownErr  error
	listeners    []net.Listener
	sync.Mutex
}

func (t *Tunnel) Start() error {
	t.Lock()
	defer t.Unlock()
	if t.shutdownChan != nil {
		return fmt.Errorf("already started")
	}
	for _, fwd := range t.ForwardAddrs {
		listener, err := t.forward(fwd)
		if err != nil {
			return err
		}
		t.listeners = append(t.listeners, listener)
	}
	t.shutdownChan = make(chan struct{})
	return nil
}

func (t *Tunnel) forward(fwd ForwardAddrs) (net.Listener, error) {
	localListener, err := net.Listen("tcp", fwd.LocalAddr)
	if err != nil {
		return nil, err
	}
	debug("listening", fwd.LocalAddr)
	go func() {
		for {
			localConn, err := localListener.Accept()
			if err != nil {
				t.Lock()
				t.shutdownErr = err
				t.Unlock()
				return
			}
			// We try several times to make the connection here to workaround
			// flakey connections that timeout. Once the connection is established
			// TCP takes care of keeping it working.
			err = retry(func() error {
				password, err := t.PasswordFunc()
				if err != nil {
					fatal(err)
				}
				cfg := &ssh.ClientConfig{
					User:            "cf:" + t.AppGuid + "/0",
					Auth:            []ssh.AuthMethod{ssh.Password(password)},
					HostKeyCallback: ssh.InsecureIgnoreHostKey(), // FIXME
				}
				debug("ssh: connecting:", cfg.User, t.TunnelAddr, fmt.Sprintf("'%s'", password))
				sshConn, err := ssh.Dial("tcp", t.TunnelAddr, cfg)
				if err != nil {
					return fmt.Errorf("error dialing ssh: %s\n", err)
				}
				debug("ssh: connected!:", cfg.User, t.TunnelAddr)
				debug("remote: connecting", fwd)
				remoteConn, err := sshConn.Dial("tcp", fwd.RemoteAddr)
				if err != nil {
					debug("remote: connection attempt failed:", err, fwd)
					return err
				}
				go copyConn(fwd, localConn, remoteConn)
				go copyConn(fwd, remoteConn, localConn)
				return nil
			})
			if err != nil {
				debug("remote: connection fail", err, fwd)
				localConn.Close()
			}
		}
	}()
	return localListener, nil
}

func (t *Tunnel) WaitChan() chan error {
	ch := make(chan error)
	go func() {
		<-t.shutdownChan
		err := t.shutdownErr
		t.Lock()
		t.shutdownErr = nil
		t.Unlock()
		ch <- err
	}()
	return ch
}

func (t *Tunnel) Stop() error {
	t.Lock()
	defer t.Unlock()
	if t.shutdownChan != nil {
		for _, listener := range t.listeners {
			listener.Close()
		}
		close(t.shutdownChan)
		t.shutdownChan = nil
	}
	return nil
}

// proxy traffic between localConn and remoteConn
func copyConn(fwd ForwardAddrs, dst, src net.Conn) {
	_, err := io.Copy(dst, src)
	if err != nil {
		if err == io.EOF {
			debug("copy failed: EOF:", fwd)
			return
		} else {
			fatal("io.Copy error", err)
		}
	}
}
