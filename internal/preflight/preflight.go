package preflight

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type CheckResult struct {
	Name    string
	OK      bool
	Message string
}

type Platform string

const (
	PlatformMacOS       Platform = "macos"
	PlatformLinuxApt    Platform = "linux-apt"
	PlatformLinuxDnf    Platform = "linux-dnf"
	PlatformLinuxPacman Platform = "linux-pacman"
	PlatformWindows     Platform = "windows"
	PlatformLinux       Platform = "linux"
)

func DetectPlatform() Platform {
	switch runtime.GOOS {
	case "darwin":
		return PlatformMacOS
	case "windows":
		return PlatformWindows
	case "linux":
		if _, err := exec.LookPath("apt"); err == nil {
			return PlatformLinuxApt
		}
		if _, err := exec.LookPath("dnf"); err == nil {
			return PlatformLinuxDnf
		}
		if _, err := exec.LookPath("pacman"); err == nil {
			return PlatformLinuxPacman
		}
		return PlatformLinux
	default:
		return Platform(runtime.GOOS)
	}
}

func CheckGitRepo(dir string) error {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil || !info.IsDir() {
		return fmt.Errorf(
			"no git repository found in %s\n\n"+
				"  Initialize a new repo:  git init\n"+
				"  Or clone an existing:   git clone <url>",
			dir,
		)
	}
	return nil
}

func CheckGhCLI() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH\n\n%s", ghInstallInstructions())
	}
	return nil
}

func CheckGhAuth() error {
	if err := CheckGhCLI(); err != nil {
		return err
	}
	cmd := exec.Command("gh", "auth", "status")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh auth check failed: %s\n\n  Run: gh auth login", string(output))
	}
	return nil
}

func CheckOpencode(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/global/health")
	if err != nil {
		return fmt.Errorf(
			"opencode not reachable at %s\n\n%s",
			url, opencodeInstallInstructions(),
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"opencode health check returned status %d at %s\n\n"+
				"  Ensure opencode serve is running",
			resp.StatusCode, url,
		)
	}
	return nil
}

func CheckOpencodeDirectory(opencodeURL, projectDir string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(opencodeURL + "/path")
	if err != nil {
		return nil // opencode not running, CheckOpencode will catch it
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil // endpoint might not exist in older versions
	}

	var result struct {
		Directory string `json:"directory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Directory == "" {
		return nil
	}

	if result.Directory != projectDir {
		return fmt.Errorf(
			"opencode serve is running in wrong directory\n"+
				"  opencode dir: %s\n"+
				"  project dir:  %s\n\n"+
				"  Stop opencode and restart it from the project directory:\n"+
				"    cd %s && opencode serve",
			result.Directory, projectDir, projectDir,
		)
	}
	return nil
}

func CheckConfig(dir string) error {
	path := filepath.Join(dir, ".oda", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf(
			"config not found at %s\n\n"+
				"  Run: oda init",
			path,
		)
	}
	return nil
}

func RunAll(projectDir, opencodeURL string) []CheckResult {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"git-repo", func() error { return CheckGitRepo(projectDir) }},
		{"gh-cli", func() error { return CheckGhCLI() }},
		{"gh-auth", func() error { return CheckGhAuth() }},
		{"opencode", func() error { return CheckOpencode(opencodeURL) }},
		{"opencode-dir", func() error { return CheckOpencodeDirectory(opencodeURL, projectDir) }},
		{"config", func() error { return CheckConfig(projectDir) }},
	}

	results := make([]CheckResult, 0, len(checks))
	for _, c := range checks {
		r := CheckResult{Name: c.name, OK: true, Message: "ok"}
		if err := c.fn(); err != nil {
			r.OK = false
			r.Message = err.Error()
		}
		results = append(results, r)
	}
	return results
}

func ghInstallInstructions() string {
	switch DetectPlatform() {
	case PlatformMacOS:
		return "  Install: brew install gh"
	case PlatformLinuxApt:
		return "  Install: sudo apt install gh"
	case PlatformLinuxDnf:
		return "  Install: sudo dnf install gh"
	case PlatformLinuxPacman:
		return "  Install: sudo pacman -S github-cli"
	case PlatformWindows:
		return "  Install: scoop install gh"
	default:
		return "  Install: see https://cli.github.com/manual/installation"
	}
}

func opencodeInstallInstructions() string {
	switch DetectPlatform() {
	case PlatformMacOS:
		return "  Install: brew install opencode-ai/tap/opencode\n" +
			"  Start:   opencode serve"
	case PlatformWindows:
		return "  Install: scoop install opencode\n" +
			"  Start:   opencode serve"
	default:
		return "  Install: curl -fsSL https://opencode.ai/install | bash\n" +
			"  Start:   opencode serve"
	}
}
