package ssh

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/alphagov/paas-cf-conduit/logging"
	"github.com/alphagov/paas-cf-conduit/util"

	"golang.org/x/crypto/ssh"
)

type ForwardAddrs struct {
	LocalPort     int64
	TLSTunnelPort int64
	RemoteAddr    string
}

const keepaliveName = "keepalive@github.com/alphagov/paas-cf-conduit"

func (f ForwardAddrs) LocalAddress() string {
	return fmt.Sprintf("localhost:%d", f.LocalPort)
}

func (f ForwardAddrs) TLSTunnelAddress() string {
	return fmt.Sprintf("localhost:%d", f.TLSTunnelPort)
}

func (f ForwardAddrs) ConnectAddress() string {
	if f.TLSTunnelPort != 0 {
		return f.TLSTunnelAddress()
	}
	return f.LocalAddress()
}

func (f ForwardAddrs) ConnectPort() int64 {
	if f.TLSTunnelPort != 0 {
		return f.TLSTunnelPort
	}
	return f.LocalPort
}

type Tunnel struct {
	TunnelAddr    string
	TunnelHostKey string
	ForwardAddrs  []ForwardAddrs
	AppGuid       string
	PasswordFunc  func() (string, error)
	shutdownChan  chan struct{}
	shutdownErr   error
	listeners     []net.Listener
	passwords     chan string
	sync.Mutex
}

func (t *Tunnel) passwordPipe() {
	if t.passwords != nil {
		return
	}
	t.passwords = make(chan string, 3)
	go func() {
		for {
			pass, err := t.PasswordFunc()
			if err != nil {
				logging.Error(err)
			}
			t.passwords <- pass
		}
	}()
}

func (t *Tunnel) Start() error {
	t.Lock()
	defer t.Unlock()
	if t.shutdownChan != nil {
		return fmt.Errorf("already started")
	}
	t.passwordPipe()
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
	localListener, err := net.Listen("tcp", fwd.LocalAddress())
	if err != nil {
		return nil, err
	}
	logging.Debug("listening", fwd.LocalAddress())
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
			err = util.Retry(func() error {
				password := <-t.passwords
				cfg := &ssh.ClientConfig{
					User: "cf:" + t.AppGuid + "/0",
					Auth: []ssh.AuthMethod{ssh.Password(password)},
					HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
						valid, possible := checkSSHFingerprint(key, t.TunnelHostKey)

						if !valid {
							return fmt.Errorf(
								"remote hostkey fingerprint %q did not match any possible values %q",
								t.TunnelHostKey, possible,
							)
						}

						return nil
					},
				}
				logging.Debug("ssh: connecting:", cfg.User, t.TunnelAddr, fmt.Sprintf("'%s'", password))
				sshConn, err := ssh.Dial("tcp", t.TunnelAddr, cfg)
				if err != nil {
					logging.Debug("ssh: connection attempt failed:", err)
					return fmt.Errorf("error dialing ssh: %s\n", err)
				}
				logging.Debug("ssh: connected!:", cfg.User, t.TunnelAddr)
				go t.startKeepalive(cfg.User, sshConn)
				logging.Debug("remote: connecting", fwd)
				remoteConn, err := sshConn.Dial("tcp", fwd.RemoteAddr)
				if err != nil {
					logging.Debug("remote: connection attempt failed:", err, fwd)
					return err
				}
				go copyConn(fwd, localConn, remoteConn)
				go copyConn(fwd, remoteConn, localConn)
				return nil
			})
			if err != nil {
				logging.Debug("remote: connection fail", err, fwd)
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

func (t *Tunnel) startKeepalive(user string, sshConnection *ssh.Client) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		if _, _, err := sshConnection.SendRequest(keepaliveName, true, make([]byte, 0)); err != nil {
			logging.Debug("failed to send keepalive message", user, t.TunnelAddr, err)
			return
		}
	}
}

// proxy traffic between localConn and remoteConn
func copyConn(fwd ForwardAddrs, dst, src net.Conn) {
	_, err := io.Copy(dst, src)
	if err != nil {
		if err == io.EOF {
			logging.Debug("copy failed: EOF:", fwd)
			return
		} else {
			logging.Error("io.Copy error", err)
		}
	}
}

func checkSSHFingerprint(expected ssh.PublicKey, actual string) (
	bool, []string,
) {
	actualWithoutColons := strings.Replace(actual, ":", "", -1)

	// sha256 fingerprints are base64 encoded, the ssh package prepends SHA256:
	// so we have to remove it
	// https://tools.ietf.org/html/rfc4648#section-3.2
	// https://github.com/golang/crypto/blob/master/ssh/keys.go#L1099
	sha256Sum := strings.Replace(
		ssh.FingerprintSHA256(expected),
		"SHA256:", "", -1,
	)

	// md5 fingerprints are hex encoded with colons between every byte
	md5Sum := strings.Replace(
		strings.Replace(ssh.FingerprintLegacyMD5(expected), "MD5:", "", -1),
		":", "", -1,
	)

	possibleVals := []string{sha256Sum, md5Sum}

	sha256Match := sha256Sum == actualWithoutColons
	md5Match := md5Sum == actualWithoutColons

	match := sha256Match || md5Match

	logging.Debug(fmt.Sprintf(
		"Fingerprint: [Match: %t ; Expected: %q ; Actual: %q ]",
		match, possibleVals, actualWithoutColons,
	))

	return match, possibleVals
}
