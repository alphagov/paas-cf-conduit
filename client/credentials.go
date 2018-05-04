package client

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Credentials interface {
	Host() string
	Port() int64
	Username() string
	Password() string
	Database() string
	URI() string
	JDBCURI() string
	SetAddress(host string, port int64)
	IsTLSEnabled() bool
}

type credentials map[string]interface{}

// Get returns with the first existing key's value (case-insensitive comparison)
func (c credentials) get(keys ...string) string {
	for _, key := range keys {
		for k, v := range c {
			if strings.EqualFold(k, key) {
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

func (c credentials) Host() string {
	return c.get("host", "hostname")
}

func (c credentials) Port() int64 {
	port, _ := strconv.Atoi(fmt.Sprintf("%v", c["port"]))
	return int64(port)
}

func (c credentials) URI() string {
	return c.get("uri", "url")
}

func (c credentials) JDBCURI() string {
	return c.get("jdbcuri", "jdbcurl", "jdbc_uri", "jdbc_url")
}

func (c credentials) Username() string {
	return c.get("user", "username")
}

func (c credentials) Password() string {
	return c.get("password", "passwd", "pwd")
}

func (c credentials) Database() string {
	return c.get("database", "db", "name")
}

func (c credentials) IsTLSEnabled() bool {
	tlsEnabled, _ := strconv.ParseBool(c.get("tls", "tls_enabled", "tlsenabled"))
	if tlsEnabled {
		return true
	}

	if strings.Contains(strings.ToLower(c.URI()), "ssl=true") {
		return true
	}
	if strings.Contains(strings.ToLower(c.JDBCURI()), "ssl=true") {
		return true
	}

	return false
}

func (c credentials) SetAddress(host string, port int64) {
	oldAddr := fmt.Sprintf("%s:%d", c.Host(), c.Port())
	newAddr := fmt.Sprintf("%s:%d", host, port)
	for k := range c {
		if stringVal, ok := c[k].(string); ok {
			c[k] = strings.Replace(stringVal, oldAddr, newAddr, -1)
		}
	}

	for k := range c {
		if strings.EqualFold(k, "host") || strings.EqualFold(k, "hostname") {
			c[k] = host
		}
	}
	c["port"] = port
}

func (c credentials) Fprint(writer io.Writer, indent string) {
	var keys []string
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(writer, "%s%s: %v\n", indent, k, c[k])
	}
}
