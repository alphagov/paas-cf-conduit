package service

import (
	"fmt"
	"github.com/alphagov/paas-cf-conduit/client"
)

type InfluxDB struct {
}

func (i InfluxDB) IsTLSEnabled(creds client.Credentials) bool {
	return false
	//return creds.IsTLSEnabled()
}

func (i InfluxDB) GetNonTLSClients() []string {
	return []string{}
}

func (i InfluxDB) GetKnownClients() []string {
	return []string{"influx", "chronograf", "telegraf", "influx_inspect", "inch"}
}

func (i InfluxDB) InitEnv(creds client.Credentials, env map[string]string) error {
	env["INFLUX_USERNAME"] = creds.Username()
	env["INFLUX_PASSWORD"] = creds.Password()
	return nil
}

func (i InfluxDB) Teardown() error {
	return nil
}

func (i InfluxDB) AdditionalProgramArgs(serviceInstances []*client.VcapService) []string {
	if len(serviceInstances) == 0 {
		return []string{}
	}

	return []string{
		"-host", serviceInstances[0].Credentials.Host(),
		"-port", fmt.Sprintf("%d", serviceInstances[0].Credentials.Port()),
		"-database", serviceInstances[0].Credentials.Database(),
		"-ssl",
		
		// Must be unsafe SSL, because the certificate presented by the Influx server
		// does not have 127.0.0.1 as a Subject Alternative Name
		"-unsafeSsl",
	}
}
