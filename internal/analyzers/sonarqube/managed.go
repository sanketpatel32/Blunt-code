package sonarqube

import (
	"bluntcode/internal/analyzers"
	"bluntcode/internal/secret"
	"bluntcode/internal/tools"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultURL = "http://127.0.0.1:9000"

// ManagedInstaller installs only the three manifest-pinned private archives.
// It never inspects PATH or a system Java installation.
type ManagedInstaller struct {
	Manager               tools.Manager
	Server, Scanner, Java tools.Artifact
	ServerRelativePath    string // retained for direct fixture construction
	ScannerRelativePath   string // retained for direct fixture construction
	Root, ToolVersion     string // retained for direct fixture construction
}

func (i ManagedInstaller) Version() string {
	if i.Server.Version != "" {
		return i.Server.Version
	}
	return i.ToolVersion
}
func (i ManagedInstaller) ServerExecutable() string {
	if i.Server.ToolID != "" {
		return i.Manager.Executable(i.Server)
	}
	return filepath.Join(i.Root, i.ServerRelativePath)
}
func (i ManagedInstaller) ScannerExecutable() string {
	if i.Scanner.ToolID != "" {
		return i.Manager.Executable(i.Scanner)
	}
	return filepath.Join(i.Root, i.ScannerRelativePath)
}
func (i ManagedInstaller) JavaHome() string {
	if i.Java.ToolID != "" {
		return filepath.Dir(filepath.Dir(i.Manager.Executable(i.Java)))
	}
	return ""
}
func (i ManagedInstaller) Ensure(ctx context.Context) error {
	if i.Server.ToolID == "" || i.Scanner.ToolID == "" || i.Java.ToolID == "" {
		return fmt.Errorf("SonarQube release is not configured: pin server, scanner, Java, and checksums in the release manifest")
	}
	for _, artifact := range []tools.Artifact{i.Server, i.Scanner, i.Java} {
		if i.Manager.IsReady(artifact) {
			continue
		}
		if err := i.Manager.InstallExecutable(ctx, artifact); err != nil {
			return fmt.Errorf("install managed %s: %w", artifact.ToolID, err)
		}
	}
	return i.Installed()
}
func (i ManagedInstaller) Installed() error {
	for label, executable := range map[string]string{"server": i.ServerExecutable(), "scanner": i.ScannerExecutable(), "Java": filepath.Join(i.JavaHome(), "bin", "java.exe")} {
		if info, err := os.Stat(executable); err != nil || info.IsDir() {
			return fmt.Errorf("managed SonarQube %s missing at %s", label, executable)
		}
	}
	return nil
}

// ManagedRuntime requires a privately managed Java runtime. It deliberately
// does not fall back to PATH or the user's JAVA_HOME.
type ManagedRuntime struct{ JavaHome string }

func (r ManagedRuntime) Validate(ctx context.Context) error {
	if r.JavaHome == "" {
		return fmt.Errorf("managed Java 21 runtime is not configured")
	}
	java := filepath.Join(r.JavaHome, "bin", "java.exe")
	if _, err := os.Stat(java); err != nil {
		return fmt.Errorf("managed Java runtime missing at %s", java)
	}
	return exec.CommandContext(ctx, java, "-version").Run()
}
func (r ManagedRuntime) Environment() map[string]string {
	return map[string]string{
		"JAVA_HOME":      r.JavaHome,
		"PATH":           filepath.Join(r.JavaHome, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"SONAR_TOKEN":    "",
		"SONAR_HOST_URL": "",
	}
}

// ManagedServer owns the child server process and only health-checks the
// configured loopback URL. It is not an external SonarQube mode.
type ManagedServer struct {
	Executable, Dir, URL string
	Args                 []string
	Environment          map[string]string
	mu                   sync.Mutex
	cmd                  *exec.Cmd
	done                 chan error
	exited               bool
	exitErr              error
	// ShutdownGracePeriod is only overridden by tests. The default lets a
	// healthy Java parent exit before its full Windows process tree is ended.
	ShutdownGracePeriod time.Duration
	terminateTree       func(context.Context, int) error
}

func (s *ManagedServer) Start(ctx context.Context, env map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		return nil
	}
	if s.Executable == "" {
		return fmt.Errorf("managed SonarQube server executable is not configured")
	}
	// The server is application-owned and shared by every scan, so it is
	// deliberately NOT bound to the starting scan's context: cancelling that
	// scan (or its timeout expiring) must not kill the server out from under
	// other in-flight scans or force a multi-minute re-bootstrap. Only
	// Shutdown ends it.
	cmd := exec.Command(s.Executable, s.Args...)
	cmd.Dir = s.Dir
	cmd.Env = append([]string(nil), os.Environ()...)
	for _, values := range []map[string]string{env, s.Environment} {
		for k, v := range values {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.exited = false
	s.exitErr = nil
	done := make(chan error, 1)
	s.done = done
	go func() {
		err := cmd.Wait()
		done <- err
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
			s.exited = true
			s.exitErr = err
		}
		s.mu.Unlock()
	}()
	return nil
}

// ExitStatus reports an unexpected child exit without consuming the process
// wait result. It lets startup fail immediately instead of waiting for the
// scan timeout when SonarQube cannot bind or clean its runtime directory.
func (s *ManagedServer) ExitStatus() (error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitErr, s.exited
}
func (s *ManagedServer) Healthy(ctx context.Context) (bool, error) {
	u := s.URL
	if u == "" {
		u = defaultURL
	}
	if err := validateLoopbackURL(u); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(u, "/")+"/api/system/status", nil)
	if err != nil {
		return false, err
	}
	res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, nil
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		return false, err
	}
	return strings.EqualFold(status.Status, "UP"), nil
}
func (s *ManagedServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	done := s.done
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// SonarQube 26 no longer exposes /api/system/shutdown. Give the owned Java
	// parent a short chance to close cleanly, then explicitly terminate its
	// process tree. Process.Kill alone leaves Elasticsearch, web, and compute
	// engine JVM children running on Windows.
	_ = cmd.Process.Signal(os.Interrupt)
	grace := s.ShutdownGracePeriod
	if grace <= 0 {
		grace = 5 * time.Second
	}
	select {
	case err := <-done:
		return err
	case <-time.After(grace):
	}
	if err := s.terminateProcessTree(ctx, cmd.Process.Pid); err != nil {
		return fmt.Errorf("stop managed SonarQube process tree: %w", err)
	}
	select {
	case <-done:
		return nil // taskkill reports a non-zero child exit by design.
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("managed SonarQube process tree did not exit")
	}
}

func (s *ManagedServer) terminateProcessTree(ctx context.Context, pid int) error {
	if s.terminateTree != nil {
		return s.terminateTree(ctx, pid)
	}
	if pid <= 0 {
		return fmt.Errorf("invalid managed SonarQube process id %d", pid)
	}
	if runtime.GOOS != "windows" {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return process.Kill()
	}
	output, err := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		text := strings.ToLower(string(output))
		if strings.Contains(text, "not found") || strings.Contains(text, "no running instance") {
			return nil
		}
		return fmt.Errorf("taskkill /T /F: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

type credentials struct {
	AdminPassword string `json:"admin_password"`
	ScannerToken  string `json:"scanner_token"`
}

// APIClient bootstraps the default local admin exactly once and persists the
// resulting credentials with Windows DPAPI. No secret reaches application logs.
type APIClient struct {
	BaseURL, ScannerToken string
	CredentialStore       secret.Store
	HTTPClient            *http.Client
	mu                    sync.Mutex
}

func (c *APIClient) Bootstrap(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := validateLoopbackURL(c.BaseURL); err != nil {
		return err
	}
	if c.ScannerToken != "" {
		return nil
	}
	stored, err := c.loadCredentials()
	if err == nil {
		c.ScannerToken = stored.ScannerToken
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	password, err := randomSecret()
	if err != nil {
		return err
	}
	if err := c.changePassword(ctx, "admin", "admin", password); err != nil {
		return fmt.Errorf("bootstrap local SonarQube: default administrator credentials are unavailable; reset only Blunt Code managed SonarQube data before retrying: %w", err)
	}
	token, err := c.generateToken(ctx, "admin", password, "blunt-code-scanner")
	if err != nil {
		return fmt.Errorf("bootstrap local SonarQube scanner token: %w", err)
	}
	if err := c.CredentialStore.Save(mustJSON(credentials{AdminPassword: password, ScannerToken: token})); err != nil {
		return fmt.Errorf("persist managed SonarQube credentials: %w", err)
	}
	c.ScannerToken = token
	return nil
}
func (c *APIClient) loadCredentials() (credentials, error) {
	b, err := c.CredentialStore.Load()
	if err != nil {
		return credentials{}, err
	}
	var value credentials
	if err := json.Unmarshal(b, &value); err != nil || value.AdminPassword == "" || value.ScannerToken == "" {
		return credentials{}, fmt.Errorf("managed SonarQube credential store is invalid")
	}
	return value, nil
}
func mustJSON(value any) []byte { b, _ := json.Marshal(value); return b }
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// SonarQube requires upper/lower case, a digit, and a special character.
	// The fixed prefix guarantees that policy without reducing random entropy.
	return "Bc1!" + base64.RawURLEncoding.EncodeToString(b), nil
}
func (c *APIClient) changePassword(ctx context.Context, login, current, next string) error {
	return c.postWithAuth(ctx, "/api/users/change_password", url.Values{"login": {login}, "previousPassword": {current}, "password": {next}}, login, current, nil)
}
func (c *APIClient) generateToken(ctx context.Context, login, password, name string) (string, error) {
	var result struct {
		Token string `json:"token"`
	}
	err := c.postWithAuth(ctx, "/api/user_tokens/generate", url.Values{"name": {name}}, login, password, &result)
	if err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("SonarQube did not return a scanner token")
	}
	return result.Token, nil
}
func (c *APIClient) Token(context.Context) (string, error) {
	if c.ScannerToken == "" {
		return "", fmt.Errorf("SonarQube scanner token is unavailable")
	}
	return c.ScannerToken, nil
}
func (c *APIClient) EnsureProject(ctx context.Context, key, name string) error {
	values := url.Values{"project": {key}, "name": {name}}
	var v any
	err := c.post(ctx, "/api/projects/create", values, &v)
	if err != nil && strings.Contains(err.Error(), "400") {
		return nil
	}
	return err
}
func (c *APIClient) WaitForTask(ctx context.Context, id string) error {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		var payload struct {
			Task struct {
				Status       string `json:"status"`
				ErrorMessage string `json:"errorMessage"`
			} `json:"task"`
		}
		if err := c.do(ctx, "/api/ce/task?id="+url.QueryEscape(id), &payload); err != nil {
			return err
		}
		switch payload.Task.Status {
		case "SUCCESS":
			return nil
		case "FAILED", "CANCELED":
			return fmt.Errorf("SonarQube compute task %s: %s", payload.Task.Status, payload.Task.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("SonarQube compute task timed out")
}
func (c *APIClient) Issues(ctx context.Context, key string) ([]Issue, error) {
	// The search endpoint caps one page at 500 issues; without walking the
	// remaining pages, any project with more than that silently under-reported.
	var all []Issue
	for page := 1; ; page++ {
		var payload apiIssueResponse
		path := fmt.Sprintf("/api/issues/search?componentKeys=%s&ps=500&p=%d", url.QueryEscape(key), page)
		if err := c.do(ctx, path, &payload); err != nil {
			return nil, err
		}
		all = append(all, payload.Issues...)
		if len(payload.Issues) < 500 {
			return all, nil
		}
	}
}
func (c *APIClient) Metrics(ctx context.Context, key string) ([]Metric, error) {
	var payload struct {
		Component struct {
			Measures []struct{ Metric, Value string } `json:"measures"`
		} `json:"component"`
	}
	const keys = "ncloc,complexity,cognitive_complexity,violations,bugs,vulnerabilities,code_smells,security_hotspots,security_rating,reliability_rating,sqale_rating,coverage,duplicated_lines_density"
	if err := c.do(ctx, "/api/measures/component?component="+url.QueryEscape(key)+"&metricKeys="+keys, &payload); err != nil {
		return nil, err
	}
	out := make([]Metric, 0, len(payload.Component.Measures))
	for _, m := range payload.Component.Measures {
		var value float64
		if _, err := fmt.Sscan(m.Value, &value); err == nil {
			out = append(out, Metric{AnalyzerID: ID, Key: m.Metric, Label: strings.ReplaceAll(m.Metric, "_", " "), Value: value})
		}
	}
	return out, nil
}
func (c *APIClient) Shutdown(ctx context.Context) error {
	return c.post(ctx, "/api/system/shutdown", nil, nil)
}
func (c *APIClient) post(ctx context.Context, endpoint string, form url.Values, v any) error {
	return c.postWithAuth(ctx, endpoint, form, c.ScannerToken, "", v)
}
func (c *APIClient) postWithAuth(ctx context.Context, endpoint string, form url.Values, user, password string, v any) error {
	if err := validateLoopbackURL(c.BaseURL); err != nil {
		return err
	}
	encoded := ""
	if form != nil {
		encoded = form.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+endpoint, strings.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(user, password)
	res, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("SonarQube API %s: %s: %s", endpoint, res.Status, bytes.TrimSpace(body))
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}
func (c *APIClient) do(ctx context.Context, endpoint string, v any) error {
	if err := validateLoopbackURL(c.BaseURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.ScannerToken, "")
	res, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("SonarQube API %s: %s", endpoint, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
func (c *APIClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func validateLoopbackURL(raw string) error {
	if raw == "" {
		raw = defaultURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
		return fmt.Errorf("SonarQube URL must be a loopback http URL")
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("SonarQube URL must be loopback")
	}
	return nil
}

// NewManaged constructs the actual local components from the exact manifest
// artifacts. The server configuration and credentials remain under dataDir.
func NewManaged(dataDir string, manager tools.Manager, manifest tools.Manifest) (*Adapter, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("managed SonarQube is supported only on Windows")
	}
	find := func(id string) (tools.Artifact, error) {
		a, ok := manifest.Find(id, runtime.GOOS+"-"+runtime.GOARCH)
		if !ok {
			return tools.Artifact{}, fmt.Errorf("managed SonarQube requires %s in the tool manifest", id)
		}
		return a, nil
	}
	serverArtifact, err := find("sonarqube-server")
	if err != nil {
		return nil, err
	}
	scannerArtifact, err := find("sonar-scanner")
	if err != nil {
		return nil, err
	}
	javaArtifact, err := find("java")
	if err != nil {
		return nil, err
	}
	installer := ManagedInstaller{Manager: manager, Server: serverArtifact, Scanner: scannerArtifact, Java: javaArtifact}
	port, err := selectLoopbackPort()
	if err != nil {
		return nil, err
	}
	databasePort, err := selectLoopbackPort()
	if err != nil {
		return nil, err
	}
	// Keep data, indexes, and credentials across app restarts. A fresh runtime
	// made every scan pay SonarQube's multi-minute database/index bootstrap.
	// Blunt Code is single-instance and ManagedServer now ends its owned process
	// tree at exit, so this directory is safe to reuse.
	runtimeDir := managedRuntimeDir(dataDir)
	if err := prepareRuntime(runtimeDir); err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serverExecutable, serverDir, serverArgs := managedServerCommand(installer)
	server := &ManagedServer{Executable: serverExecutable, Dir: serverDir, Args: serverArgs, URL: baseURL, Environment: managedServerEnvironment(runtimeDir, port, databasePort)}
	client := &APIClient{BaseURL: baseURL, CredentialStore: secret.Store{Path: filepath.Join(runtimeDir, "credentials.dpapi")}}
	a := New(installer, ManagedRuntime{JavaHome: installer.JavaHome()}, server, client)
	a.DataDir, a.BaseURL = dataDir, baseURL
	return a, nil
}

func managedRuntimeDir(dataDir string) string {
	return filepath.Join(dataDir, "sonarqube", "managed-runtime")
}

func managedServerEnvironment(runtimeDir string, webPort, databasePort int) map[string]string {
	return map[string]string{
		"SONAR_PATH_DATA":             filepath.Join(runtimeDir, "data"),
		"SONAR_PATH_TEMP":             filepath.Join(runtimeDir, "temp"),
		"SONAR_PATH_LOGS":             filepath.Join(runtimeDir, "logs"),
		"SONAR_WEB_HOST":              "127.0.0.1",
		"SONAR_WEB_PORT":              fmt.Sprintf("%d", webPort),
		"SONAR_EMBEDDEDDATABASE_PORT": fmt.Sprintf("%d", databasePort),
		"SONAR_SEARCH_PORT":           "0",
		"SONAR_TELEMETRY_ENABLE":      "false",
		"SONAR_UPDATECENTER_ACTIVATE": "false",
	}
}

// managedServerCommand bypasses StartSonar.bat. A batch file is only a
// wrapper around Java and exits independently of its child process on
// Windows, which made a healthy server look as if it had stopped. Launching
// the pinned Java runtime directly gives ManagedServer ownership of the real
// SonarQube process.
func managedServerCommand(installer ManagedInstaller) (string, string, []string) {
	launcher := installer.ServerExecutable()
	serverRoot := filepath.Dir(filepath.Dir(filepath.Dir(launcher)))
	java := filepath.Join(installer.JavaHome(), "bin", "java.exe")
	jar := filepath.Join(serverRoot, "lib", "sonar-application-"+installer.Version()+".jar")
	args := []string{
		"-Djava.awt.headless=true",
		"--add-exports=java.base/jdk.internal.ref=ALL-UNNAMED",
		"--add-opens=java.base/java.lang=ALL-UNNAMED",
		"--add-opens=java.base/java.nio=ALL-UNNAMED",
		"--add-opens=java.base/sun.nio.ch=ALL-UNNAMED",
		"--add-opens=java.management/sun.management=ALL-UNNAMED",
		"--add-opens=jdk.management/com.sun.management.internal=ALL-UNNAMED",
		"-cp", jar,
		"org.sonar.application.App",
	}
	return java, serverRoot, args
}

func selectLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve loopback SonarQube port: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if port < 1024 {
		return 0, fmt.Errorf("selected invalid privileged SonarQube port %d", port)
	}
	return port, nil
}

func writeServerConfig(runtimeDir, configDir string, port int) (string, error) {
	if runtimeDir == "" {
		return "", fmt.Errorf("Blunt Code SonarQube runtime directory is required")
	}
	if port < 1024 || port > 65535 {
		return "", fmt.Errorf("SonarQube loopback port is invalid")
	}
	root := runtimeDir
	if configDir == "" {
		return "", fmt.Errorf("managed SonarQube configuration directory is required")
	}
	configDir = filepath.Clean(configDir)
	if err := prepareRuntime(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", err
	}
	propertyPath := func(value string) string { return filepath.ToSlash(value) }
	properties := strings.Join([]string{
		"sonar.web.host=127.0.0.1", fmt.Sprintf("sonar.web.port=%d", port),
		"sonar.path.data=" + propertyPath(filepath.Join(root, "data")), "sonar.path.temp=" + propertyPath(filepath.Join(root, "temp")), "sonar.path.logs=" + propertyPath(filepath.Join(root, "logs")),
		"sonar.telemetry.enable=false", "sonar.updatecenter.activate=false",
	}, "\n") + "\n"
	path := filepath.Join(configDir, "sonar.properties")
	if err := os.WriteFile(path, []byte(properties), 0o600); err != nil {
		return "", err
	}
	return configDir, nil
}

func prepareRuntime(root string) error {
	if root == "" {
		return fmt.Errorf("Blunt Code SonarQube runtime directory is required")
	}
	for _, dir := range []string{filepath.Join(root, "data"), filepath.Join(root, "temp"), filepath.Join(root, "logs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown closes the private, owned server process tree when the application
// exits. The SonarQube 26 system shutdown endpoint is unavailable.
func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.Server != nil {
		return a.Server.Shutdown(ctx)
	}
	return nil
}

var _ analyzers.Analyzer = (*Adapter)(nil)
