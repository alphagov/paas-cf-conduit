package service

import (
	"fmt"
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

func (r *Redis) AdditionalProgramArgs(serviceInstances []*client.VcapService) []string {
	if len(serviceInstances) == 0 {
		return []string{}
	}

	return []string{
		"-h", serviceInstances[0].Credentials.Host(),
		"-p", fmt.Sprintf("%d", serviceInstances[0].Credentials.Port()),
		"-a", serviceInstances[0].Credentials.Password(),
	}
}
