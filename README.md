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
`wezspawn [--wezterm /path/to/wezterm] [--gui-app /path/to/WezTerm.app] [command...]` uses the focused GUI workspace.
When no GUI is attached, it starts one. CLI queries use `--no-auto-start` to avoid invisible headless windows.

## Verify

Run `nix flake check -L`. Checks cover SSH options, native domains, local overrides, Mosh refusal, and CLI focus selection.

Extracted from [sysinit.wezterm](https://github.com/roshbhatia/sysinit.wezterm) at `266b285205309f3fa1174612a97669f4b3c1eee0`.
See LICENSE for the original license.
