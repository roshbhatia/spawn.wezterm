# spawn.wezterm

Open WezTerm tabs, splits, and windows on the intended host.

The Lua plugin retains native mux domains and reconstructs SSH or Mosh connections for new shells.
The `wezspawn` CLI opens a window in the focused workspace and directory.

## Install the plugin

```lua
local wezterm = require("wezterm")
local spawn = wezterm.plugin.require("https://github.com/roshbhatia/spawn.wezterm")
local config = wezterm.config_builder()
spawn.apply_to_config(config, {
  keys = {
    { key = "s", mods = "ALT", kind = "Down" },
    { key = "v", mods = "ALT", kind = "Right" },
    { key = "t", mods = "ALT", kind = "tab" },
    { key = "t", mods = "ALT|SHIFT", kind = "tab", local_host = true },
  },
})
return config
```

Keys are optional. Use `spawn.spawn(kind, local_host)` to create an action for your own bindings.
Set `commands = { ssh = "/path/to/ssh", mosh = "/path/to/mosh" }` when PATH is unavailable.
Set `home` to override the local working directory. Nix is optional.

SSH actions preserve connection options and clear forwarding and remote-command settings.
They do not replay the command running in the source pane.
Ambiguous Mosh connections return an error instead of silently spawning locally.
A forced local split uses the local domain and home directory.

## Install the CLI

```sh
nix profile add github:roshbhatia/spawn.wezterm#wezspawn
wezspawn --help
```

With Go installed, `go install ./cmd/wezspawn` builds the CLI from a checkout.
`wezspawn [--wezterm /path/to/wezterm] [--gui-app /path/to/WezTerm.app] [--cwd /path] [--domain name] [command...]` uses the most recently active client with a focused pane, retaining its workspace and directory.
If a GUI has no open windows, the launcher can still open a window without inheriting a directory.
When no GUI is attached, it starts one and forwards the requested command and arguments to the first window.
The macOS `--gui-app` path uses Launch Services; other launches use `wezterm start`.
`--cwd` and `--domain` override the inherited directory and domain in either case.
CLI queries use `--no-auto-start` to avoid invisible headless windows.
Failed queries and malformed replies return an error rather than opening a second GUI.
A missing or refused implicit Unix socket permits a cold launch; a failed explicit `WEZTERM_UNIX_SOCKET` does not.
Warm launches print the pane ID; cold launches return after starting the GUI without printing an ID.

## Verify

Run `nix flake check -L`. Checks cover SSH options, native domains, local overrides, Mosh refusal, and CLI focus selection.

Extracted from [sysinit.wezterm](https://github.com/roshbhatia/sysinit.wezterm) at `266b285205309f3fa1174612a97669f4b3c1eee0`.
See LICENSE for the original license.
