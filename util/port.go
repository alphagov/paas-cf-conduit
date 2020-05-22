package util

import (
	"fmt"
	"net"
	"strconv"
)

func PortIsInUse(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))

	if listener != nil {
		listener.Close()
	}

	if err == nil {
		// port is not in use, because we could listen on it
		return false
	}

	return true
}

func GetRandomPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		return 0, err
	}

	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(port)
}
