package wezspawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Summary = "open a WezTerm window in the focused workspace and directory"

const usage = `wezspawn: open a WezTerm window where the focused pane already is

Usage:
  wezspawn [--wezterm <path>] [--gui-app <path>] [--cwd <path>] [--domain <name>] [prog...]

The new window joins the workspace the GUI has focused and starts in the focused
pane's directory, so a spawn from a window manager lands in the session the user is
looking at rather than in the default one. prog runs instead of the shell.

With no GUI attached, wezspawn opens the GUI and forwards prog to its first window.
CLI calls never start a headless mux. Query failures are reported unless the
implicit local socket is confirmed unavailable; a failed explicit socket is not replaced.

--wezterm names the binary to drive, for a caller with none on PATH. A window manager
is one, so it passes its own store path rather than relying on a shell to prepend it.
--gui-app names the macOS bundle to open for that GUI; without it the wezterm binary
runs directly. --cwd and --domain override the inherited directory and default
domain for both warm and cold launches.

Prints the new pane's id when using an existing GUI. A cold start returns after
launching the GUI without printing an id. Exits 1 when a query or launch fails.
`

var muxTimeout = 5 * time.Second
var queryMux = muxOutput
var startGUI = launch

var errNoMux = errors.New("no local mux is listening")
var failedSocket = regexp.MustCompile(`failed to connect to Socket\(("(?:[^"\\]|\\.)*")\)`)

type client struct {
	Workspace     string `json:"workspace"`
	FocusedPaneID *int   `json:"focused_pane_id"`
	IdleTime      *struct {
		Secs  uint64 `json:"secs"`
		Nanos uint32 `json:"nanos"`
	} `json:"idle_time"`
}

type pane struct {
	PaneID int    `json:"pane_id"`
	Cwd    string `json:"cwd"`
}

type target struct {
	workspace      string
	cwd            string
	domain         string
	defaultStartup bool
}

func configuredDomain() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(root, "wezterm", "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var config struct {
		LocalDomain string `json:"localDomain"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("read WezTerm local domain: %w", err)
	}
	return config.LocalDomain, nil
}

func localPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return ""
	}
	info, err := os.Stat(parsed.Path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return parsed.Path
}

func focus(clients []client, panesJSON string) (target, error) {
	var selected *client
	for i := range clients {
		c := &clients[i]
		if c.IdleTime == nil || c.IdleTime.Nanos >= 1_000_000_000 {
			return target{}, fmt.Errorf("client has an invalid idle_time")
		}
		if selected != nil && selected.FocusedPaneID != nil && c.FocusedPaneID == nil {
			continue
		}
		if selected == nil || (selected.FocusedPaneID == nil && c.FocusedPaneID != nil) ||
			c.IdleTime.Secs < selected.IdleTime.Secs ||
			(c.IdleTime.Secs == selected.IdleTime.Secs && c.IdleTime.Nanos < selected.IdleTime.Nanos) {
			selected = c
		}
	}
	if selected == nil {
		return target{}, fmt.Errorf("the GUI has no focused pane")
	}
	if selected.FocusedPaneID == nil {
		return target{workspace: selected.Workspace}, nil
	}
	var panes []pane
	if err := json.Unmarshal([]byte(panesJSON), &panes); err != nil {
		return target{}, fmt.Errorf("decode pane list: %w", err)
	}
	for _, p := range panes {
		if p.PaneID == *selected.FocusedPaneID {
			return target{workspace: selected.Workspace, cwd: localPath(p.Cwd)}, nil
		}
	}
	return target{}, fmt.Errorf("focused pane %d is absent from the pane list", *selected.FocusedPaneID)
}

func launch(bin, app string, found target, prog []string) error {
	args := []string{"start", "--always-new-process"}
	if found.defaultStartup && found.cwd == "" && len(prog) == 0 {
		args = nil
	}
	if found.domain != "" && args != nil {
		args = append(args, "--domain", found.domain)
	}
	if found.cwd != "" {
		args = append(args, "--cwd", found.cwd)
	}
	if len(prog) > 0 {
		args = append(append(args, "--"), prog...)
	}
	if app != "" {
		args = append([]string{"-n", "-a", app, "--args"}, args...)
		out, err := exec.Command("open", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("open GUI: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return exec.Command(bin, args...).Start()
}

func spawnArgs(found target, prog []string) []string {
	args := []string{"cli", "--no-auto-start", "spawn", "--new-window"}
	if found.workspace != "" {
		args = append(args, "--workspace", found.workspace)
	}
	if found.domain != "" {
		args = append(args, "--domain-name", found.domain)
	}
	if found.cwd != "" {
		args = append(args, "--cwd", found.cwd)
	}
	if len(prog) > 0 {
		args = append(args, "--")
		args = append(args, prog...)
	}
	return args
}

func muxOutput(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), muxTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = muxTimeout
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("no answer in %s", muxTimeout)
	}
	if err == nil {
		return string(out), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		diagnostic := strings.TrimSpace(string(exit.Stderr))
		if os.Getenv("WEZTERM_UNIX_SOCKET") == "" {
			// The CLI has no typed connection error, so verify its socket before opening another GUI.
			if match := failedSocket.FindStringSubmatch(diagnostic); match != nil {
				if path, parseErr := strconv.Unquote(match[1]); parseErr == nil {
					conn, dialErr := net.DialTimeout("unix", path, time.Second)
					if dialErr == nil {
						conn.Close()
					} else if errors.Is(dialErr, os.ErrNotExist) || errors.Is(dialErr, syscall.ECONNREFUSED) {
						return "", fmt.Errorf("%w: %s", errNoMux, diagnostic)
					}
				}
			}
			if strings.Contains(diagnostic, "no default unix domain is configured and WEZTERM_UNIX_SOCKET is not set in the environment") {
				return "", fmt.Errorf("%w: %s", errNoMux, diagnostic)
			}
		}
		return "", fmt.Errorf("%w: %s", err, diagnostic)
	}
	return "", err
}

func Run(args []string) int {
	bin := "wezterm"
	app := ""
	overrides := target{}
	for len(args) > 0 && (args[0] == "--wezterm" || args[0] == "--gui-app" || args[0] == "--cwd" || args[0] == "--domain") {
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "wezspawn: %s needs a value\n", args[0])
			return 2
		}
		switch args[0] {
		case "--wezterm":
			bin = args[1]
		case "--gui-app":
			app = args[1]
		case "--cwd":
			overrides.cwd = args[1]
		case "--domain":
			overrides.domain = args[1]
		}
		args = args[2:]
	}
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Print(usage)
			return 0
		}
	}
	if overrides.domain == "" {
		domain, err := configuredDomain()
		if err != nil {
			fmt.Fprintf(os.Stderr, "wezspawn: %v\n", err)
			return 1
		}
		overrides.domain = domain
		overrides.defaultStartup = domain != "" && domain != "local"
	}

	clientsJSON, err := queryMux(bin, "cli", "--no-auto-start", "list-clients", "--format", "json")
	var clients []client
	if err == nil {
		err = json.Unmarshal([]byte(clientsJSON), &clients)
		if err != nil {
			err = fmt.Errorf("decode client list: %w", err)
		} else if clients == nil {
			err = fmt.Errorf("decode client list: expected an array")
		}
	}
	if errors.Is(err, errNoMux) || (err == nil && len(clients) == 0) {
		if err := startGUI(bin, app, overrides, args); err != nil {
			fmt.Fprintf(os.Stderr, "wezspawn: %v\n", err)
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "wezspawn: %v\n", err)
		return 1
	}
	panes, err := queryMux(bin, "cli", "--no-auto-start", "list", "--format", "json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "wezspawn: list panes: %v\n", err)
		return 1
	}
	found, err := focus(clients, panes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wezspawn: %v\n", err)
		return 1
	}
	if overrides.cwd != "" {
		found.cwd = overrides.cwd
	}
	found.domain = overrides.domain
	out, err := queryMux(bin, spawnArgs(found, args)...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wezspawn: %v\n", err)
		return 1
	}
	fmt.Print(out)
	return 0
}
