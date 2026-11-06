package wezspawn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredPersistentDomain(t *testing.T) {
	calls, _ := stubMux(t, clientsFor("work", 2), panesFor(2, t.TempDir()))
	root := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(root, "wezterm"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wezterm", "config.json"), []byte(`{"localDomain":"unix"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run(nil); code != 0 {
		t.Fatalf("Run: %d", code)
	}
	args := (*calls)[len(*calls)-1].args
	found := false
	for i, value := range args {
		if value == "--domain-name" && args[i+1] == "unix" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spawn escaped persistent mux: %q", args)
	}
	queryMux = func(string, ...string) (string, error) { return "[]", nil }
	startGUI = func(_ string, _ string, target target, _ []string) error {
		if !target.defaultStartup || target.domain != "unix" {
			t.Fatalf("cold startup bypassed recovery: %+v", target)
		}
		return nil
	}
	if code := Run(nil); code != 0 {
		t.Fatal(code)
	}
}

func TestInvalidDomainConfigurationStopsLaunch(t *testing.T) {
	calls, launched := stubMux(t, "[]", "[]")
	root := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(root, "wezterm"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wezterm", "config.json"), []byte(`{"localDomain":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run(nil); code != 1 {
		t.Fatalf("Run: %d", code)
	}
	if *launched || len(*calls) != 0 {
		t.Fatal("malformed configuration caused a launch")
	}
}
