package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alphagov/paas-cf-conduit/client"
	"github.com/alphagov/paas-cf-conduit/logging"
)

type MySQL struct {
	workDir    string
	serviceCnt int
}

func (m *MySQL) IsTLSEnabled(creds client.Credentials) bool {
	return creds.IsTLSEnabled()
}

func (m *MySQL) InitEnv(creds client.Credentials, env map[string]string) error {
	// We will only set the configuration for the first service
	if m.serviceCnt > 0 {
		return nil
	}

	var err error
	m.workDir, err = ioutil.TempDir("", "conduit")
	if err != nil {
		return err
	}
	mycnfPath := filepath.Join(m.workDir, "my.cnf")
	mycnf := "[mysql]\n"
	mycnf += fmt.Sprintf("user = %s\n", creds.Username())
	mycnf += fmt.Sprintf("password = %s\n", creds.Password())
	mycnf += fmt.Sprintf("host = %s\n", creds.Host())
	mycnf += fmt.Sprintf("port = %d\n", creds.Port())
	mycnf += fmt.Sprintf("database = %s\n", creds.Database())
	mycnf += "[mysqldump]\n"
	mycnf += fmt.Sprintf("user = %s\n", creds.Username())
	mycnf += fmt.Sprintf("password = %s\n", creds.Password())
	mycnf += fmt.Sprintf("host = %s\n", creds.Host())
	mycnf += fmt.Sprintf("port = %d\n", creds.Port())
	if err := ioutil.WriteFile(mycnfPath, []byte(mycnf), 0644); err != nil {
		return fmt.Errorf("failed to create temporary mysql config: %s", err)
	}
	env["MYSQL_HOME"] = m.workDir
	m.serviceCnt++

	return nil
}

func (m *MySQL) Teardown() error {
	if m.workDir != "" {
		logging.Debug("deleting", m.workDir)
		return os.RemoveAll(m.workDir)
	}
	return nil
}

func (m *MySQL) GetNonTLSClients() []string {
	return []string{}
}

func (m *MySQL) GetKnownClients() []string {
	return []string{"mysql", "mysqldump"}
}

func (m *MySQL) AdditionalProgramArgs(serviceInstances []*client.VcapService) []string {
	return []string{}
}
