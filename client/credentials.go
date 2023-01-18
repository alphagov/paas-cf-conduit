package client

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Credentials map[string]interface{}

// Get returns with the first existing key's value (case-insensitive comparison)
func (c *Credentials) get(keys ...string) string {
	for _, key := range keys {
		for k, v := range *c {
			if strings.EqualFold(k, key) {
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

func (c *Credentials) Host() string {
	return c.get("host", "hostname")
}

func (c *Credentials) Port() int64 {
	port, _ := strconv.Atoi(fmt.Sprintf("%v", (*c)["port"]))
	return int64(port)
}

func (c *Credentials) URI() string {
	return c.get("uri", "url")
}

func (c *Credentials) JDBCURI() string {
	return c.get("jdbcuri", "jdbcurl", "jdbc_uri", "jdbc_url")
}

func (c *Credentials) Username() string {
	return c.get("user", "username")
}

func (c *Credentials) Password() string {
	return c.get("password", "passwd", "pwd")
}

func (c *Credentials) Database() string {
	return c.get("database", "db", "name")
}

func (c *Credentials) IsTLSEnabled() bool {
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

func (c *Credentials) SetAddress(host string, port int64) {
	oldAddr := fmt.Sprintf("%s:%d", c.Host(), c.Port())
	newAddr := fmt.Sprintf("%s:%d", host, port)
	for k := range *c {
		if stringVal, ok := (*c)[k].(string); ok {
			(*c)[k] = strings.Replace(stringVal, oldAddr, newAddr, -1)
		}
	}

	for k := range *c {
		if strings.EqualFold(k, "host") || strings.EqualFold(k, "hostname") {
			(*c)[k] = host
		}
	}
	(*c)["port"] = fmt.Sprintf("%d", port)
}

func (c *Credentials) Fprint(writer io.Writer, indent string) {
	var keys []string
	for k := range *c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(writer, "%s%s: %v\n", indent, k, (*c)[k])
	}
}
