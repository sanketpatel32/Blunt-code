package sonarqube

import (
	"bluntcode/internal/analyzers"
	"bluntcode/internal/secret"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type exitedServer struct{}

func (exitedServer) Start(context.Context, map[string]string) error { return nil }
func (exitedServer) Healthy(context.Context) (bool, error)          { return false, nil }
func (exitedServer) Shutdown(context.Context) error                 { return nil }
func (exitedServer) ExitStatus() (error, bool)                      { return errors.New("exit status 1"), true }

type startupClient struct{}

func (startupClient) Bootstrap(context.Context) error                     { return nil }
func (startupClient) EnsureProject(context.Context, string, string) error { return nil }
func (startupClient) Token(context.Context) (string, error)               { return "", nil }
func (startupClient) WaitForTask(context.Context, string) error           { return nil }
func (startupClient) Issues(context.Context, string) ([]Issue, error)     { return nil, nil }
func (startupClient) Metrics(context.Context, string) ([]Metric, error)   { return nil, nil }

func TestEnsureRunningFailsImmediatelyWhenServerExits(t *testing.T) {
	adapter := &Adapter{Server: exitedServer{}, Client: startupClient{}, Runtime: ManagedRuntime{JavaHome: "configured"}, StartupTimeout: time.Minute}
	if err := adapter.EnsureRunning(context.Background()); err == nil || !strings.Contains(err.Error(), "stopped during startup") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestIssueFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "sonarqube", "issues.json"))
	if err != nil {
		t.Fatal(err)
	}
	var response apiIssueResponse
	if err := json.Unmarshal(b, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Issues) != 1 {
		t.Fatal("expected one issue")
	}
	f := response.Issues[0].Finding()
	if f.Severity != analyzers.SeverityHigh || f.Category != analyzers.CategoryVulnerability || f.RelativePath != "test:src/main.ts" {
		t.Fatalf("unexpected finding %#v", f)
	}
}

func TestBootstrapCreatesDPAPIProtectedScannerToken(t *testing.T) {
	var changed, generated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/users/change_password":
			user, pass, ok := r.BasicAuth()
			if !ok || user != "admin" || pass != "admin" {
				t.Errorf("unexpected bootstrap auth")
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "previousPassword=admin") {
				t.Errorf("expected prior password")
			}
			changed = true
			w.WriteHeader(http.StatusNoContent)
		case "/api/user_tokens/generate":
			user, pass, ok := r.BasicAuth()
			if !ok || user != "admin" || pass == "admin" {
				t.Errorf("expected rotated admin credentials")
			}
			generated = true
			_, _ = w.Write([]byte(`{"token":"scanner-token"}`))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.dpapi")
	client := &APIClient{BaseURL: server.URL, CredentialStore: secret.Store{Path: path}}
	if err := client.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !changed || !generated || client.ScannerToken != "scanner-token" {
		t.Fatalf("bootstrap did not create managed credentials")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "scanner-token") {
		t.Fatal("token was stored in plaintext")
	}
	client.ScannerToken = ""
	if err := client.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.ScannerToken != "scanner-token" {
		t.Fatal("expected persisted DPAPI token")
	}
}

func TestRandomSecretMeetsSonarPasswordPolicy(t *testing.T) {
	value, err := randomSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(value, "Bc1!") || len(value) < 20 {
		t.Fatalf("unexpected generated password shape")
	}
}

func TestScannerFailureOutputRedactsToken(t *testing.T) {
	output := scannerFailureOutput("ERROR scanner failed\nsonar.token=private-value\nmore detail")
	if strings.Contains(output, "private-value") || !strings.Contains(output, "sonar.token=***") {
		t.Fatalf("scanner output was not redacted: %q", output)
	}
}

func TestManagedRuntimeClearsExternalSonarSettings(t *testing.T) {
	env := (ManagedRuntime{JavaHome: `C:\managed-java`}).Environment()
	if env["SONAR_TOKEN"] != "" || env["SONAR_HOST_URL"] != "" {
		t.Fatalf("managed runtime must clear Sonar settings: %#v", env)
	}
}

func TestManagedServerRejectsNonLoopbackEndpoint(t *testing.T) {
	server := &ManagedServer{URL: "http://example.com:9000"}
	if _, err := server.Healthy(context.Background()); err == nil {
		t.Fatal("non-loopback SonarQube endpoint was accepted")
	}
}

func TestManagedServerShutdownTerminatesOwnedProcessTreeAfterGracePeriod(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process tree behavior")
	}
	cmd := exec.Command("cmd.exe", "/c", "timeout /t 30 /nobreak >NUL")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	terminated := 0
	server := &ManagedServer{
		cmd: cmd, done: done, ShutdownGracePeriod: time.Millisecond,
		terminateTree: func(_ context.Context, pid int) error {
			terminated++
			if pid != cmd.Process.Pid {
				t.Fatalf("unexpected process id %d", pid)
			}
			return cmd.Process.Kill()
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if terminated != 1 {
		t.Fatalf("expected one tree termination, got %d", terminated)
	}
}

func TestTerminateProcessTreeStopsManagedChildOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process tree behavior")
	}
	cmd := exec.Command("cmd.exe", "/c", "timeout /t 30 /nobreak >NUL")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (&ManagedServer{}).terminateProcessTree(ctx, cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("taskkill did not end the owned child")
	}
}

func TestManagedServerCommandUsesPinnedJavaInsteadOfBatchWrapper(t *testing.T) {
	installer := ManagedInstaller{
		Root:               `C:\tools\sonarqube`,
		ServerRelativePath: `sonarqube-26.8\bin\windows-x86-64\StartSonar.bat`,
		ToolVersion:        "26.8",
	}
	executable, dir, args := managedServerCommand(installer)
	if executable != `bin\java.exe` {
		t.Fatalf("unexpected executable %q", executable)
	}
	if dir != `C:\tools\sonarqube\sonarqube-26.8` {
		t.Fatalf("unexpected server directory %q", dir)
	}
	if len(args) == 0 || strings.Contains(strings.Join(args, " "), "StartSonar.bat") || !strings.Contains(strings.Join(args, " "), "sonar-application-26.8.jar") {
		t.Fatalf("unexpected server command %q", args)
	}
}

func TestServerConfigUsesSelectedLoopbackPort(t *testing.T) {
	port, err := selectLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	if port < 1024 {
		t.Fatalf("expected unprivileged port, got %d", port)
	}
	config, err := writeServerConfig(t.TempDir(), filepath.Join(t.TempDir(), "conf"), port)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(config, "sonar.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "sonar.web.host=127.0.0.1") || !strings.Contains(string(b), "sonar.web.port="+strconv.Itoa(port)) || strings.Contains(string(b), `\`) {
		t.Fatalf("server config does not bind chosen loopback port: %s", b)
	}
}

func TestManagedServerEnvironmentUsesSeparatePrivateDatabasePort(t *testing.T) {
	env := managedServerEnvironment(`C:\BluntCode\runtime`, 51234, 51235)
	if env["SONAR_WEB_PORT"] != "51234" || env["SONAR_EMBEDDEDDATABASE_PORT"] != "51235" {
		t.Fatalf("managed SonarQube ports were not isolated: %#v", env)
	}
}

func TestManagedRuntimeDirectoryIsStableAcrossRestarts(t *testing.T) {
	root := `C:\BluntCode`
	first := managedRuntimeDir(root)
	second := managedRuntimeDir(root)
	if first != second || !strings.HasSuffix(first, `sonarqube\managed-runtime`) {
		t.Fatalf("managed runtime must be stable and private: %q / %q", first, second)
	}
}

func TestScannerPropertiesRemainInAppData(t *testing.T) {
	root := t.TempDir()
	p, cleanup, err := ScannerProperties(root, `C:\workspace`, "bluntcode:one", "http://127.0.0.1:9000", "secret", []string{"vendor/**"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("properties escaped app data: %s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), "sonar.projectKey=bluntcode:one") || strings.Contains(string(b), `\`) {
		t.Fatal("properties missing expected values")
	}
}
