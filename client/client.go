package client

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
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
	Credentials *Credentials
}

type Credentials struct {
	Host     string `json:"host"`
	Port     int64  `json:"port"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	URI      string `json:"uri"`
	JDBCURI  string `json:"jdbcuri"`
}

func (c *Credentials) SetAddress(host string, port int64) {
	oldAddr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	newAddr := fmt.Sprintf("%s:%d", host, port)
	c.URI = strings.Replace(c.URI, oldAddr, newAddr, 1)
	c.JDBCURI = strings.Replace(c.JDBCURI, oldAddr, newAddr, 1)
	c.Host = host
	c.Port = port
}

type jsonCredentials struct {
	Host     string      `json:"host"`
	Port     json.Number `json:"port"`
	Name     string      `json:"name"`
	Username string      `json:"username"`
	Password string      `json:"password"`
	URI      string      `json:"uri"`
	JDBCURI  string      `json:"jdbcuri"`
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
	httpClient, err := newHttpClient(info.AuthEndpoint, info.TokenEndpoint, token, insecure)
	if err != nil {
		return nil, err
	}
	c := &Client{
		HttpClient:         httpClient,
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
	err := c.fetch("GET", "/v2/apps/"+appGuid+"/env", nil, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (c *Client) GetSpaceByName(orgGuid string, name string) (*Space, error) {
	spaces, err := c.GetSpaces(fmt.Sprintf("name:%s", name), fmt.Sprintf("organization_guid:%s", orgGuid))
	if err != nil {
		return nil, err
	}
	for _, space := range spaces {
		if space.Name == name && space.OrgGuid == orgGuid {
			return space, nil
		}
	}
	return nil, fmt.Errorf("no space named '%s' in org %s", name, orgGuid)
}

func (c *Client) GetSpaces(filters ...string) (map[string]*Space, error) {
	uri, err := url.Parse("/v2/spaces")
	if err != nil {
		return nil, err
	}
	q := uri.Query()
	for _, filter := range filters {
		q.Add("q", filter)
	}
	uri.RawQuery = q.Encode()
	resources, err := c.getResources(uri.String())
	if err != nil {
		return nil, err
	}
	entities := map[string]*Space{}
	for _, r := range resources {
		var entity Space
		err := json.Unmarshal(r.RawEntity, &entity)
		if err != nil {
			return nil, err
		}
		entity.Guid = r.Metadata.Guid
		entity.CreatedAt = r.Metadata.CreatedAt
		entity.UpdatedAt = r.Metadata.UpdatedAt
		entities[entity.Guid] = &entity
	}
	return entities, nil
}

func (c *Client) GetOrgByName(name string) (*Org, error) {
	orgs, err := c.GetOrgs(fmt.Sprintf("name:%s", name))
	if err != nil {
		return nil, err
	}
	for _, org := range orgs {
		if org.Name == name {
			return org, nil
		}
	}
	return nil, fmt.Errorf("no org named '%s'", name)
}

func (c *Client) GetOrgs(filters ...string) (map[string]*Org, error) {
	uri, err := url.Parse("/v2/organizations")
	if err != nil {
		return nil, err
	}
	q := uri.Query()
	for _, filter := range filters {
		q.Add("q", filter)
	}
	uri.RawQuery = q.Encode()
	resources, err := c.getResources(uri.String())
	if err != nil {
		return nil, err
	}
	entities := map[string]*Org{}
	for _, r := range resources {
		var entity Org
		err := json.Unmarshal(r.RawEntity, &entity)
		if err != nil {
			return nil, err
		}
		entity.Guid = r.Metadata.Guid
		entity.CreatedAt = r.Metadata.CreatedAt
		entity.UpdatedAt = r.Metadata.UpdatedAt
		entities[entity.Guid] = &entity
	}
	return entities, nil
}

func (c *Client) GetServiceInstances(filters ...string) (map[string]*ServiceInstance, error) {
	uri, err := url.Parse("/v2/service_instances")
	if err != nil {
		return nil, err
	}
	q := uri.Query()
	for _, filter := range filters {
		q.Add("q", filter)
	}
	uri.RawQuery = q.Encode()
	resources, err := c.getResources(uri.String())
	if err != nil {
		return nil, err
	}
	services := map[string]*ServiceInstance{}
	for _, r := range resources {
		var service ServiceInstance
		err := json.Unmarshal(r.RawEntity, &service)
		if err != nil {
			return nil, err
		}
		service.Guid = r.Metadata.Guid
		service.CreatedAt = r.Metadata.CreatedAt
		service.UpdatedAt = r.Metadata.UpdatedAt
		services[service.Guid] = &service
	}
	return services, nil
}

func (c *Client) BindService(appGuid string, serviceInstanceGuid string) (*Credentials, error) {
	res := struct {
		Entity struct {
			Credentials jsonCredentials `json:"credentials"`
		} `json:"entity"`
	}{}
	err := c.fetch("POST", "/v2/service_bindings", map[string]interface{}{
		"app_guid":              appGuid,
		"service_instance_guid": serviceInstanceGuid,
	}, &res)
	if err != nil {
		return nil, err
	}
	port, _ := res.Entity.Credentials.Port.Int64()
	creds := &Credentials{
		Host:     res.Entity.Credentials.Host,
		Port:     port,
		Name:     res.Entity.Credentials.Name,
		Username: res.Entity.Credentials.Username,
		Password: res.Entity.Credentials.Password,
		URI:      res.Entity.Credentials.URI,
		JDBCURI:  res.Entity.Credentials.JDBCURI,
	}
	return creds, nil

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
