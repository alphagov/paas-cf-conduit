package service

import (
	"strings"

	"github.com/alphagov/paas-cf-conduit/client"
)

type Redis struct {
}

func (r *Redis) IsTLSEnabled(creds client.Credentials) bool {
	return creds.IsTLSEnabled() || strings.HasPrefix(creds.URI(), "rediss")
}

func (r *Redis) InitEnv(creds client.Credentials, env map[string]string) error {
	return nil
}

func (r *Redis) Teardown() error {
	return nil
}

func (r *Redis) GetNonTLSClients() []string {
	return []string{"redis-cli"}
}

func (r *Redis) GetKnownClients() []string {
	return []string{"redis-cli"}
}
