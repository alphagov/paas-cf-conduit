package util

import (
	"fmt"
	"net"
	"time"

	"github.com/alphagov/paas-cf-conduit/logging"
)

func WaitForConnection(addr string) chan error {
	timeout := 3 * time.Second
	connection := make(chan error)
	go func() {
		defer close(connection)
		tries := 0
		for {
			if tries > 5 {
				time.Sleep(2 * time.Second)
			} else {
				time.Sleep(1 * time.Second)
			}
			tries++
			logging.Debug("waiting for", addr, "attempt", tries)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				if tries < 15 {
					continue
				}
				connection <- fmt.Errorf("connection fail after %d attempts: %s", tries, err)
				break
			}
			defer conn.Close()
			connection <- nil
			break
		}
	}()
	return connection
}
