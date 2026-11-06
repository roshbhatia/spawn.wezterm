local actions = setmetatable({}, {
  __index = function(_, name)
    return function(value)
      return { [name] = value }
    end
  end,
})
package.preload.wezterm = function()
  return {
    action = actions,
    action_callback = function(fn)
      return fn
    end,
    home_dir = "/home/test",
  }
end
local plugin = dofile(assert(arg[1]))
local config = { keys = { { key = "existing" } } }
plugin.apply_to_config(
  config,
  { commands = { ssh = "/bin/ssh" }, keys = {
    { key = "s", mods = "ALT", kind = "Down" },
  } }
)
assert(#config.keys == 2)
local argv = assert(plugin.ssh({ "ssh", "-p", "2222", "-L", "8000:localhost:80", "host", "remote-command" }))
assert(table.concat(argv, " ") == "/bin/ssh -o ClearAllForwardings=yes -o RemoteCommand=none -p 2222 host")
assert(plugin.ssh({ "ssh", "-p" }) == nil)
local pane = {
  get_foreground_process_info = function()
    return { executable = "ssh", argv = { "ssh", "host" } }
  end,
}
local spawn = assert(plugin.action(pane, "tab", false)).SpawnCommandInNewTab
assert(spawn.args[#spawn.args] == "host")
assert(spawn.domain.DomainName == "local")
local native = assert(plugin.action({}, "Right", false)).SplitPane.command
assert(native.domain == "CurrentPaneDomain")
local local_spawn = assert(plugin.action(pane, "Down", true)).SplitPane.command
assert(local_spawn.cwd == "/home/test" and local_spawn.args == nil)
assert(plugin.remote({ executable = "mosh-client", argv = { "mosh-client", "ambiguous" } }) == nil)
local executed
config.keys[2].action({
  perform_action = function(_, action)
    executed = action
  end,
}, pane)
assert(executed.SplitPane.command.args[#executed.SplitPane.command.args] == "host")
print("spawn plugin tests passed")
plugin.apply_to_config({}, { local_domain = "unix", home = "/home/test" })
assert(plugin.action(pane, "Down", true).SplitPane.command.domain.DomainName == "unix")
assert(plugin.action(pane, "tab", false).SpawnCommandInNewTab.domain.DomainName == "unix")
