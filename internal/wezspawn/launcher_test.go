package wezspawn

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunSelectsMostRecentlyActiveFocusedClient(t *testing.T) {
	dir := t.TempDir()
	clients := `[
 {"workspace":"old","focused_pane_id":1,"idle_time":{"secs":5,"nanos":0}},
 {"workspace":"recent","focused_pane_id":2,"idle_time":{"secs":0,"nanos":2}},
 {"workspace":"newer","focused_pane_id":3,"idle_time":{"secs":0,"nanos":1}},
 {"workspace":"unfocused","focused_pane_id":null,"idle_time":{"secs":0,"nanos":0}}
 ]`
	calls, launched := stubMux(t, clients, panesFor(3, dir))
	if code := Run(nil); code != 0 {
		t.Fatalf("Run exited %d", code)
	}
	last := (*calls)[len(*calls)-1]
	want := []string{"cli", "--no-auto-start", "spawn", "--new-window", "--workspace", "newer", "--cwd", dir}
	if *launched || !reflect.DeepEqual(last.args, want) {
		t.Fatalf("launched=%v args=%q", *launched, last.args)
	}
}

func TestRunAllowsGUIWithNoOpenWindows(t *testing.T) {
	calls, launched := stubMux(t, `[{"workspace":"empty","focused_pane_id":null,"idle_time":{"secs":0,"nanos":0}}]`, `[]`)
	if code := Run(nil); code != 0 {
		t.Fatalf("Run exited %d", code)
	}
	last := (*calls)[len(*calls)-1]
	if *launched || !reflect.DeepEqual(last.args, []string{"cli", "--no-auto-start", "spawn", "--new-window", "--workspace", "empty"}) {
		t.Fatalf("launched=%v args=%q", *launched, last.args)
	}
}

func TestRunDoesNotLaunchOrSpawnAfterQueryFailure(t *testing.T) {
	for _, tc := range []struct{ name, clients, panes, failedQuery string }{
		{"client error", "", "", "list-clients"},
		{"pane error", clientsFor("review", 2), "", "list"},
		{"invalid clients", "not JSON", "", ""},
		{"null clients", "null", "", ""},
		{"invalid idle time", `[{"workspace":"x","focused_pane_id":2}]`, panesFor(2, "/tmp"), ""},
		{"invalid panes", clientsFor("review", 2), "not JSON", ""},
		{"missing focused pane", clientsFor("review", 2), "[]", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, launched := stubMux(t, tc.clients, tc.panes)
			original := queryMux
			queryMux = func(bin string, args ...string) (string, error) {
				if args[2] == tc.failedQuery {
					return "", errors.New("query failed")
				}
				return original(bin, args...)
			}
			if code := Run(nil); code != 1 {
				t.Fatalf("Run exited %d", code)
			}
			if *launched {
				t.Fatal("query failure launched another GUI")
			}
			for _, call := range *calls {
				if call.args[2] == "spawn" {
					t.Fatal("query failure spawned a window")
				}
			}
		})
	}
}

func TestRunForwardsColdCommand(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(strconv.FormatBool(unavailable), func(t *testing.T) {
			stubMux(t, "[]", "[]")
			if unavailable {
				queryMux = func(string, ...string) (string, error) { return "", errNoMux }
			}
			var got []string
			startGUI = func(bin, app string, found target, prog []string) error {
				if bin != "/tool/wezterm" || app != "/Applications/WezTerm.app" {
					t.Fatalf("wrong launcher: %q %q", bin, app)
				}
				got = append([]string(nil), prog...)
				return nil
			}
			want := []string{"/bin/echo", "two words", "--flag"}
			args := append([]string{"--wezterm", "/tool/wezterm", "--gui-app", "/Applications/WezTerm.app"}, want...)
			if code := Run(args); code != 0 {
				t.Fatalf("Run exited %d", code)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("prog=%q want=%q", got, want)
			}
		})
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("WEZSPAWN_HELPER") != "" {
		if path := os.Getenv("WEZSPAWN_ARGS_FILE"); path != "" {
			data, _ := json.Marshal(os.Args[1:])
			if err := os.WriteFile(path, data, 0600); err != nil {
				os.Exit(2)
			}
		}
		fmt.Fprint(os.Stdout, os.Getenv("WEZSPAWN_STDOUT"))
		fmt.Fprint(os.Stderr, os.Getenv("WEZSPAWN_STDERR"))
		code, _ := strconv.Atoi(os.Getenv("WEZSPAWN_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestMuxOutputOnlyClassifiesUnavailableImplicitSockets(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "wezspawn-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, diagnostic, explicit string
		listening, stale, noMux    bool
	}{
		{"missing socket", fmt.Sprintf("failed to connect to Socket(%q): connecting; terminating", path), "", false, false, true},
		{"live socket query failure", fmt.Sprintf("failed to connect to Socket(%q): timeout", path), "", true, false, false},
		{"explicit socket", fmt.Sprintf("failed to connect to Socket(%q): connecting", path), path, false, false, false},
		{"refused socket", fmt.Sprintf("failed to connect to Socket(%q): connecting", path), "", false, true, true},
		{"permission denied", "permission denied", "", false, false, false},
		{"protocol mismatch", "codec version mismatch", "", false, false, false},
		{"no configured endpoint", "no default unix domain is configured and WEZTERM_UNIX_SOCKET is not set in the environment", "", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.listening || tc.stale {
				l, err := net.Listen("unix", path)
				if err != nil {
					t.Fatal(err)
				}
				if tc.stale {
					l.(*net.UnixListener).SetUnlinkOnClose(false)
					l.Close()
				} else {
					defer l.Close()
				}
			}
			t.Setenv("WEZTERM_UNIX_SOCKET", tc.explicit)
			t.Setenv("WEZSPAWN_HELPER", "1")
			t.Setenv("WEZSPAWN_STDERR", tc.diagnostic)
			t.Setenv("WEZSPAWN_EXIT", "1")
			_, err := muxOutput(binary)
			if err == nil || errors.Is(err, errNoMux) != tc.noMux {
				t.Fatalf("error=%v noMux=%v", err, tc.noMux)
			}
			if !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("lost diagnostic: %v", err)
			}
		})
	}
}

func TestLaunchForwardsArgumentsThroughOpen(t *testing.T) {
	dir := t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, filepath.Join(dir, "open")); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "args.json")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEZSPAWN_HELPER", "1")
	t.Setenv("WEZSPAWN_ARGS_FILE", argsFile)
	for _, prog := range [][]string{nil, {"/bin/echo", "two words", "--flag"}} {
		if err := launch("unused", "/Applications/WezTerm.app", target{domain: "local", cwd: "/a directory"}, prog); err != nil {
			t.Fatal(err)
		}
		var got []string
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		want := []string{"-a", "/Applications/WezTerm.app", "--args", "start", "--domain", "local", "--cwd", "/a directory"}
		if len(prog) > 0 {
			want = append(append(want, "--"), prog...)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("open args=%q want=%q", got, want)
		}
	}
	t.Setenv("WEZSPAWN_EXIT", "1")
	t.Setenv("WEZSPAWN_STDERR", "Launch Services refused")
	if err := launch("unused", "/Applications/WezTerm.app", target{}, nil); err == nil || !strings.Contains(err.Error(), "Launch Services refused") {
		t.Fatalf("open failure=%v", err)
	}
}

func TestLaunchForwardsArgumentsWithoutApp(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "args.json")
	t.Setenv("WEZSPAWN_HELPER", "1")
	t.Setenv("WEZSPAWN_ARGS_FILE", path)
	want := []string{"start", "--", "/bin/echo", "two words"}
	if err := launch(binary, "", target{}, want[2:]); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		data, err := os.ReadFile(path)
		if err == nil {
			var got []string
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("GUI args=%q want=%q", got, want)
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("GUI helper did not receive command arguments")
}

func TestRunPreservesExplicitDomainAndDirectory(t *testing.T) {
	for _, cold := range []bool{false, true} {
		t.Run(strconv.FormatBool(cold), func(t *testing.T) {
			clients := clientsFor("review", 2)
			if cold {
				clients = "[]"
			}
			calls, launched := stubMux(t, clients, panesFor(2, "/tmp"))
			original := startGUI
			startGUI = func(bin, app string, found target, prog []string) error {
				if found.domain != "local" || found.cwd != "/a directory" {
					t.Fatalf("cold context=%+v", found)
				}
				return original(bin, app, found, prog)
			}
			if code := Run([]string{"--cwd", "/a directory", "--domain", "local", "/bin/echo", "test"}); code != 0 {
				t.Fatalf("exit=%d", code)
			}
			if cold {
				if !*launched {
					t.Fatal("cold GUI not launched")
				}
				return
			}
			if *launched {
				t.Fatal("warm launch started another GUI")
			}
			want := []string{"cli", "--no-auto-start", "spawn", "--new-window", "--workspace", "review", "--domain-name", "local", "--cwd", "/a directory", "--", "/bin/echo", "test"}
			if got := (*calls)[len(*calls)-1].args; !reflect.DeepEqual(got, want) {
				t.Fatalf("spawn=%q want=%q", got, want)
			}
		})
	}
}
