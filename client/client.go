package client

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	gocfclient "github.com/cloudfoundry-community/go-cfclient"
)

type Env struct {
	SystemEnv *SystemEnv `json:"system_env_json"`
}

type SystemEnv struct {
	VcapServices map[string][]*VcapService `json:"VCAP_SERVICES"`
}

type VcapService struct {
	Name         string
	Credentials  credentials
	InstanceName string `json:"instance_name"`
}

func NewClient(api string, token string, insecure bool, cipherSuites []uint16, minTLSVersion uint16) (*Client, error) {

	// Use the TLS config when creating an HTTP client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
				MinVersion:         minTLSVersion,
				CipherSuites:       cipherSuites,
			},
		},
	}

	clientConfig := &gocfclient.Config{
		ApiAddress: api,
		Token:      strings.TrimPrefix(token, "bearer "),
		HttpClient: client,
	}
	cf, err := gocfclient.NewClient(clientConfig)
	if err != nil {
		return nil, err
	}

	info, err := cf.GetInfo()
	if err != nil {
		return nil, err
	}

	c := &Client{
		CFClient:           cf,
		ApiEndpoint:        api,
		InsecureSkipVerify: insecure,
		CipherSuites:       cipherSuites,
		MinTLSVersion:      minTLSVersion,
		Info:               info,
		Token:              token,
	}

	return c, nil
}

type Client struct {
	Verbose            bool
	CFClient           gocfclient.CloudFoundryClient
	ApiEndpoint        string
	InsecureSkipVerify bool
	CipherSuites       []uint16
	MinTLSVersion      uint16
	Token              string
	Info               *gocfclient.Info
}

func (c *Client) GetNewAccessToken() (string, error) {
	token, err := c.output("oauth-token")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimPrefix(token, "bearer ")), nil
}

func (c *Client) GetAppEnv(appGuid string) (*Env, error) {
	env := Env{}
	req := c.CFClient.NewRequest("GET", "/v2/apps/"+appGuid+"/env")
	resp, err := c.CFClient.DoRequest(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(body, &env)

	return &env, nil
}

func (c *Client) GetSpaceByName(orgGuid string, name string) (*gocfclient.Space, error) {
	space, err := c.CFClient.GetSpaceByName(name, orgGuid)

	if err != nil {
		return nil, err
	}

	return &space, err
}

func (c *Client) GetOrgByName(name string) (*gocfclient.Org, error) {
	org, err := c.CFClient.GetOrgByName(name)

	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (c *Client) GetAppByName(orgGuid, spaceGuid, appName string) (*gocfclient.App, error) {
	app, err := c.CFClient.AppByName(appName, spaceGuid, orgGuid)

	if err != nil {
		return nil, err
	}
	return &app, nil
}

func (c *Client) GetServiceBindings(filters ...string) (map[string]*gocfclient.ServiceBinding, error) {
	svcBindingMap := map[string]*gocfclient.ServiceBinding{}

	bindings, err := c.CFClient.ListServiceBindingsByQuery(url.Values{
		"q": filters,
	})

	if err != nil {
		return svcBindingMap, err
	}

	for i, binding := range bindings {
		svcBindingMap[binding.ServiceInstanceGuid] = &bindings[i]
	}

	return svcBindingMap, nil
}

func (c *Client) GetServiceInstances(filters ...string) (map[string]*gocfclient.ServiceInstance, error) {
	svcInstanceMap := map[string]*gocfclient.ServiceInstance{}

	instances, err := c.CFClient.ListServiceInstancesByQuery(url.Values{
		"q": filters,
	})

	if err != nil {
		return svcInstanceMap, err
	}

	for i, instance := range instances {
		svcInstanceMap[instance.Guid] = &instances[i]
	}

	return svcInstanceMap, nil
}

func (c *Client) BindService(
	appGuid string,
	serviceInstanceGuid string,
	parameters map[string]interface{},
) (Credentials, error) {
	res := struct {
		Entity struct {
			Credentials credentials `json:"credentials"`
		} `json:"entity"`
	}{}

	body := map[string]interface{}{
		"app_guid":              appGuid,
		"service_instance_guid": serviceInstanceGuid,
		"parameters":            parameters,
	}
	bodyJson, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req := c.CFClient.NewRequestWithBody("POST", "/v2/service_bindings", bytes.NewReader(bodyJson))
	resp, err := c.CFClient.DoRequest(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respBytes, &res)
	if err != nil {
		return nil, err
	}

	return res.Entity.Credentials, nil

}

func (c *Client) UploadStaticAppBits(appGuid string) error {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	err := createZeroByteFileInZip(zipWriter, "Staticfile")
	if err != nil {
		return err
	}

	return c.CFClient.UploadAppBits(buf, appGuid)
}

func (c *Client) DestroyApp(appGuid string) error {
	req := c.CFClient.NewRequest("DELETE", "/v3/apps/"+appGuid)
	_, err := c.CFClient.DoRequest(req)
	return err
}

func (c *Client) CreateApp(name string, spaceGUID string) (guid string, err error) {
	req := gocfclient.AppCreateRequest{
		Name:            name,
		SpaceGuid:       spaceGUID,
		EnableSSH:       true,
		Instances:       1,
		Memory:          64,
		DiskQuota:       256,
		Buildpack:       "staticfile_buildpack",
		HealthCheckType: "none",
		Diego:           true,
	}

	app, err := c.CFClient.CreateApp(req)
	if err != nil {
		return "", err
	}

	return app.Guid, nil
}

func (c *Client) StartApp(appGuid string) error {
	return c.CFClient.StartApp(appGuid)
}

func (c *Client) PollForAppState(appGuid string, state string, maxRetries int) error {
	tries := 0
	for {
		app, err := c.CFClient.GetAppByGuid(appGuid)
		if err != nil {
			return err
		}
		if app.State == state {
			return nil
		}
		tries++
		if tries > maxRetries {
			return fmt.Errorf("timeout waiting for GET app %s to return %s state", app.Guid, state)
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *Client) output(args ...string) (string, error) {
	cmd, err := c.newCommand(args...)
	if err != nil {
		return "", err
	}
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) newCommand(args ...string) (*exec.Cmd, error) {
	exe, err := exec.LookPath("cf")
	if err != nil {
		return nil, errors.New("cannot find 'cf' command in PATH")
	}
	cxt := context.Background()
	cmd := exec.CommandContext(cxt, exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Env = os.Environ()
	return cmd, nil
}

func (c *Client) SSHCode() (string, error) {
	// Uses its own http client as the token endpoint is on a different domain
	errPreventRedirect := errors.New("prevent-redirect")
	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return errPreventRedirect
		},
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: c.InsecureSkipVerify,
				CipherSuites:       c.CipherSuites,
				MinVersion:         c.MinTLSVersion,
			},
			Proxy:               http.ProxyFromEnvironment,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	info, err := c.CFClient.GetInfo()
	if err != nil {
		return "", err
	}

	authorizeURL, err := url.Parse(info.TokenEndpoint)
	if err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", info.AppSSHOauthClient)

	authorizeURL.Path = "/oauth/authorize"
	authorizeURL.RawQuery = values.Encode()

	authorizeReq, err := http.NewRequest("GET", authorizeURL.String(), nil)
	if err != nil {
		return "", err
	}

	token, err := c.CFClient.GetToken()
	if err != nil {
		return "", err
	}

	authorizeReq.Header.Add("authorization", token)

	resp, err := httpClient.Do(authorizeReq)
	if err == nil {
		return "", errors.New("Authorization server did not redirect with one time code")
	}
	if netErr, ok := err.(*url.Error); !ok || netErr.Err != errPreventRedirect {
		return "", fmt.Errorf("Error requesting one time code from server: %v", err)
	}

	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("Error getting the redirected location: %v", err)
	}

	codes := loc.Query()["code"]
	if len(codes) != 1 {
		return "", fmt.Errorf("Unable to acquire one time code from authorization response")
	}

	return codes[0], nil
}

func createZeroByteFileInZip(zipWriter *zip.Writer, name string) error {
	fileInZip, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = fileInZip.Write([]byte{})
	if err != nil {
		return err
	}

	err = zipWriter.Close()
	if err != nil {
		return err
	}
	return nil
}
