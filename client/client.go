package client

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alphagov/paas-cf-conduit/logging"
	gocfclient "github.com/cloudfoundry-community/go-cfclient"

	"golang.org/x/oauth2"
)

type Metadata struct {
	Guid      string    `json:"guid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type resource struct {
	Metadata  Metadata        `json:"metadata"`
	RawEntity json.RawMessage `json:"entity"`
}
type page struct {
	TotalPages   int        `json:"total_pages"`
	TotalResults int        `json:"total_results"`
	Resources    []resource `json:"resources"`
}

type Info struct {
	DopplerEndpoint   string `json:"doppler_logging_endpoint"`
	LoggingEndpoint   string `json:"logging_endpoint"`
	AuthEndpoint      string `json:"authorization_endpoint"`
	TokenEndpoint     string `json:"token_endpoint"`
	AppSshEndpoint    string `json:"app_ssh_endpoint"`
	AppSshHostKey     string `json:"app_ssh_host_key_fingerprint"`
	AppSshOauthClient string `json:"app_ssh_oauth_client"`
}

type Env struct {
	SystemEnv *SystemEnv `json:"system_env_json"`
}

type SystemEnv struct {
	VcapServices map[string][]*VcapService `json:"VCAP_SERVICES"`
}

type VcapService struct {
	Name        string
	Credentials credentials
}

type Org struct {
	Guid                string    `json:"guid"`
	UpdatedAt           time.Time `json:"updated_at"`
	CreatedAt           time.Time `json:"created_at"`
	Name                string    `json:"name"`
	QuotaDefinitionGuid string    `json:"quota_definition_guid"`
	Status              string    `json:"status"`
}

type Space struct {
	Guid      string    `json:"guid"`
	OrgGuid   string    `json:"organization_guid"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
}

type ServiceInstance struct {
	Guid            string    `json:"guid"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedAt       time.Time `json:"created_at"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	ServicePlanGuid string    `json:"service_plan_guid"`
	SpaceGuid       string    `json:"space_guid"`
	// Space           *Space            `json:"-"`
	// Bindings        []*ServiceBinding `json:"-"`
	// Plan            *ServicePlan      `json:"-"`
}

func newHttpClient(authEndpoint string, tokenEndpoint string, token string, insecure bool) (*http.Client, error) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
			TLSHandshakeTimeout:   http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
			ExpectContinueTimeout: http.DefaultTransport.(*http.Transport).ExpectContinueTimeout,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	}
	authConfig := &oauth2.Config{
		ClientID: "cf",
		Scopes:   []string{""},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authEndpoint + "/oauth/auth",
			TokenURL: tokenEndpoint + "/oauth/token",
		},
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	tokenSource := authConfig.TokenSource(ctx, &oauth2.Token{
		AccessToken: strings.TrimPrefix(token, "bearer "),
		TokenType:   "Bearer",
	})
	return oauth2.NewClient(ctx, tokenSource), nil
}

func getInfo(api string, insecure bool) (*Info, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
			TLSHandshakeTimeout:   http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
			ExpectContinueTimeout: http.DefaultTransport.(*http.Transport).ExpectContinueTimeout,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	}
	var info Info
	resp, err := httpClient.Get(api + "/v2/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return nil, err
	}
	return &info, err
}

func NewClient(api string, token string, insecure bool) (*Client, error) {
	info, err := getInfo(api, insecure)
	if err != nil {
		return nil, err
	}
	clientConfig := &gocfclient.Config{
		ApiAddress: api,
		Token:      strings.TrimPrefix(token, "bearer "),
	}
	cf, _ := gocfclient.NewClient(clientConfig)

	httpClient, err := newHttpClient(info.AuthEndpoint, info.TokenEndpoint, token, insecure)
	if err != nil {
		return nil, err
	}

	c := &Client{
		HttpClient:         httpClient,
		CFClient:           cf,
		ApiEndpoint:        api,
		InsecureSkipVerify: insecure,
		Info:               info,
		Token:              token,
	}

	return c, nil
}

type Client struct {
	Verbose            bool
	HttpClient         *http.Client
	CFClient           gocfclient.CloudFoundryClient
	ApiEndpoint        string
	InsecureSkipVerify bool
	Token              string
	Info               *Info
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

func (c *Client) GetServiceInstances(filters ...string) (map[string]*gocfclient.ServiceInstance, error) {
	svcInstanceMap := map[string]*gocfclient.ServiceInstance{}

	instances, err := c.CFClient.ListServiceInstancesByQuery(url.Values{
		"q": filters,
	})

	if err != nil {
		return svcInstanceMap, err
	}

	for i, instance := range instances {
		svcInstanceMap[instance.Guid] = &instances[i];
	}
	
	return svcInstanceMap, nil
}

func (c *Client) BindService(appGuid string, serviceInstanceGuid string) (Credentials, error) {
	res := struct {
		Entity struct {
			Credentials credentials `json:"credentials"`
		} `json:"entity"`
	}{}
	err := c.fetch("POST", "/v2/service_bindings", map[string]interface{}{
		"app_guid":              appGuid,
		"service_instance_guid": serviceInstanceGuid,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Entity.Credentials, nil

}

func (c *Client) UploadStaticAppBits(appGuid string) error {
	buf := new(bytes.Buffer)
	zipFile := zip.NewWriter(buf)
	_, err := zipFile.Create("Staticfile")
	if err != nil {
		return err
	}

	return c.UploadAppBits(appGuid, buf)
}

func (c *Client) UploadAppBits(appGuid string, bits io.Reader) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	err := writer.WriteField("resources", `[{"fn":"Staticfile","size":0,"sha1":"da39a3ee5e6b4b0d3255bfef95601890afd80709","mode":"644"}]`)
	if err != nil {
		return err
	}
	err = writer.WriteField("async", "true")
	if err != nil {
		return err
	}
	part, err := writer.CreateFormFile("application", "application.zip")
	if err != nil {
		return err
	}
	_, err = io.Copy(part, bits)
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	uri := "/v2/apps/" + appGuid + "/bits"
	req, err := c.NewRequest("PUT", uri, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "go-cli 6.28.0 / "+runtime.GOOS)
	req.ContentLength = int64(body.Len())
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 && res.StatusCode != 201 {
		resBody, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("%s %s: request to send %d bytes failed with status %d: %s", req.Method, uri, req.ContentLength, res.StatusCode, string(resBody))
	}
	return nil

}

func (c *Client) DestroyApp(appGuid string) error {
	return c.fetch("DELETE", "/v3/apps/"+appGuid, nil, nil)
}

func (c *Client) CreateApp(name string, spaceGUID string) (guid string, err error) {
	req := map[string]interface{}{
		"name":              name,
		"space_guid":        spaceGUID,
		"enable_ssh":        true,
		"instances":         1,
		"memory":            64,
		"disk_quota":        256,
		"buildpack":         "staticfile_buildpack",
		"health_check_type": "none",
		"diego":             true,
	}
	res := resource{}
	if err := c.fetch("POST", "/v2/apps", req, &res); err != nil {
		return "", err
	}
	if res.Metadata.Guid == "" {
		return "", fmt.Errorf("assertion failure: expected an app guid returned after CreateApp")
	}
	return res.Metadata.Guid, nil
}

func (c *Client) UpdateApp(appGuid string, req map[string]interface{}) error {
	res := resource{}
	return c.fetch("PUT", "/v2/apps/"+appGuid, req, &res)
}

func (c *Client) PollForAppState(appGuid string, state string, maxRetries int) error {
	res := struct {
		State string `json:"state"`
	}{}
	method := "GET"
	uri := "/v3/apps/" + appGuid
	tries := 0
	for {
		if err := c.fetch(method, uri, nil, &res); err != nil {
			return err
		}
		if res.State == state {
			return nil
		}
		tries++
		if tries > maxRetries {
			return fmt.Errorf("timeout waiting for %s %s to return %s state", method, uri, state)
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *Client) NewRequest(method string, apipath string, body io.Reader) (*http.Request, error) {
	uri := c.ApiEndpoint + apipath
	return http.NewRequest(method, uri, body)
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.HttpClient.Do(req)
}

func (c *Client) getResources(path string) ([]resource, error) {
	resultsPerPage := 50
	pages := make(chan *page, 10)
	// get first page
	var p page
	err := c.fetch("GET", set(path, 1, resultsPerPage), nil, &p)
	if err != nil {
		return nil, err
	}
	// extact total number of pages
	totalPages := p.TotalPages
	pages <- &p
	// fill channel with requests for rest of pages
	errs := make(chan error)
	var wg sync.WaitGroup
	if totalPages > 1 {
		for i := 2; i < totalPages+1; i++ {
			wg.Add(1)
			go func(uri string) {
				defer wg.Done()
				var p page
				err := c.fetch("GET", uri, nil, &p)
				if err != nil {
					errs <- err
					return
				}
				pages <- &p
			}(set(path, i, resultsPerPage))
		}
	}
	go func() {
		wg.Wait()
		close(pages)
		close(errs)
	}()
	// collect errors
	es := []error{}
	for err := range errs {
		es = append(es, err)
	}
	if len(es) > 0 {
		return nil, err
	}
	// collect resources
	resources := []resource{}
	for p := range pages {
		resources = append(resources, p.Resources...)
	}
	return resources, nil
}

func (c *Client) fetch(method string, apipath string, requestData interface{}, resposneData interface{}) error {
	var body bytes.Buffer
	if method == "POST" || method == "PUT" {
		err := json.NewEncoder(&body).Encode(requestData)
		if err != nil {
			return err
		}
	}
	logging.Debug(method, apipath)
	req, err := c.NewRequest(method, apipath, &body)
	if err != nil {
		return err
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 202 {
		resBody, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("%s %s: request failed with status %d: %s", req.Method, apipath, res.StatusCode, string(resBody))
	}
	if resposneData != nil {
		if res.Body == nil {
			return fmt.Errorf("%s %s: request failed with status %d and no response body", req.Method, apipath, res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&resposneData); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) pushAppWithoutRoute(appName string, appDir string) error {
	return c.run("push", appName,
		"-p", appDir,
		"-b", "staticfile_buildpack",
		"-m", "64M",
		"-k", "64M",
		"-i", "1",
		"--health-check-type", "none",
		"--no-route",
		"--no-manifest",
	)
}

func (c *Client) getAppGUID(appName string) (string, error) {
	appGUIDLines, err := c.output("app", "--guid", appName)
	if err != nil {
		return "", err
	}
	appGUID := strings.TrimSpace(appGUIDLines)
	if appGUID == "" {
		return "", fmt.Errorf("Expected app ID for '%s' was empty.", appName)
	}
	return appGUID, nil
}

func (c *Client) getAppEnv(appGUID string) (string, error) {
	return c.output("curl", "/v2/apps/"+appGUID+"/env")
}

func (c *Client) bindService(appName string, serviceInstanceName string) error {
	return c.run("bind-service", appName, serviceInstanceName)
}

func (c *Client) forceDeleteApp(appName string) error {
	return c.run("delete", "-f", appName)
}

func (c *Client) forceDeleteAppWithRetries(appName string, tries int) (err error) {
	for try := 0; try < tries; try++ {
		err = c.forceDeleteApp(appName)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return err
}

func (c *Client) sshPortForward(appName string, localPort int64, remoteHost string, remotePort int64) (*exec.Cmd, error) {
	portSpec := fmt.Sprintf("%d:%s:%d",
		localPort,
		remoteHost,
		remotePort,
	)
	return c.newCommand("ssh", appName, "-L", portSpec, "-N")
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

func (c *Client) run(args ...string) error {
	cmd, err := c.newCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
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

func set(path string, n int, limit int) string {
	page, err := url.Parse(path)
	if err != nil {
		log.Fatal(err)
	}
	q := page.Query()
	q.Set("page", fmt.Sprintf("%d", n))
	if limit > 0 {
		q.Set("results-per-page", fmt.Sprintf("%d", limit))
	}
	page.RawQuery = q.Encode()
	return page.String()
}

func (c *Client) SSHCode() (string, error) {
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
			},
			Proxy:               http.ProxyFromEnvironment,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	authorizeURL, err := url.Parse(c.Info.TokenEndpoint)
	if err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", c.Info.AppSshOauthClient)

	authorizeURL.Path = "/oauth/authorize"
	authorizeURL.RawQuery = values.Encode()

	authorizeReq, err := http.NewRequest("GET", authorizeURL.String(), nil)
	if err != nil {
		return "", err
	}

	authorizeReq.Header.Add("authorization", c.Token)

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

func getBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil

}
