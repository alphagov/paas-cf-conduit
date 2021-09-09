package service

import (
	"fmt"

	"github.com/alphagov/paas-cf-conduit/client"
)

type Postgres struct {
	serviceCnt int
}

func (p *Postgres) IsTLSEnabled(creds client.Credentials) bool {
	return creds.IsTLSEnabled()
}

func (p *Postgres) InitEnv(creds client.Credentials, env map[string]string) error {
	// We will only set the configuration for the first service
	if p.serviceCnt > 0 {
		return nil
	}

	env["PGDATABASE"] = creds.Database()
	env["PGHOST"] = creds.Host()
	env["PGPORT"] = fmt.Sprintf("%d", creds.Port())
	env["PGUSER"] = creds.Username()
	env["PGPASSWORD"] = creds.Password()

	p.serviceCnt++
	return nil
}

func (p *Postgres) Teardown() error {
	return nil
}

func (p *Postgres) GetNonTLSClients() []string {
	return []string{}
}

func (p *Postgres) GetKnownClients() []string {
	return []string{"psql", "pg_dump"}
}

func (p *Postgres) AdditionalProgramArgs(serviceInstances []*client.VcapService) []string {
	return []string{}
}
