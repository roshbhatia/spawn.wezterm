local wezterm = require("wezterm")
local M = {}

local function basename(value)
  return (value or ""):match("([^/]+)$") or ""
end

local options = {}

local function executable(name)
  return (options.commands or {})[name] or name
end

function M.apply_to_config(config, opts)
  options = opts or {}
  config.keys = config.keys or {}
  for _, binding in ipairs(options.keys or {}) do
    config.keys[#config.keys + 1] = {
      key = binding.key,
      mods = binding.mods,
      action = M.spawn(binding.kind, binding.local_host),
    }
  end
end

function M.spawn(kind, local_host)
  return wezterm.action_callback(function(window, pane)
    local action, err = M.action(pane, kind, local_host)
    if action then
      window:perform_action(action, pane)
    else
      window:toast_notification("spawn.wezterm", err, nil, 5000)
    end
  end)
end

function M.ssh(argv)
  local command = { executable("ssh"), "-o", "ClearAllForwardings=yes", "-o", "RemoteCommand=none" }
  local values = "BbcDEeFIiJLlmOoPpQRSWw"
  local retain = "bciFJlmpS"
  local i = 2
  while i <= #argv do
    local arg = argv[i]
    if arg == "--" then
      i = i + 1
      break
    end
    if arg:sub(1, 1) ~= "-" then
      break
    end
    local flag = arg:sub(2, 2)
    if values:find(flag, 1, true) then
      local value = arg:sub(3)
      if value == "" then
        i = i + 1
        value = argv[i]
      end
      if not value then
        return nil, "Cannot read the SSH connection options"
      end
      if retain:find(flag, 1, true) or flag == "o" then
        command[#command + 1] = "-" .. flag
        command[#command + 1] = value
      end
    elseif arg:match("^%-[46AaKkqv]+$") then
      command[#command + 1] = arg
    elseif not arg:match("^%-[tTnNfXxYyCMgs]+$") then
      return nil, "Cannot read the SSH connection options"
    end
    i = i + 1
  end
  local host = argv[i]
  if not host or host == "" or host:sub(1, 1) == "-" then
    return nil, "Cannot find the SSH destination"
  end
  command[#command + 1] = host
  return command
end

function M.remote(info)
  if not info then
    return nil
  end
  local argv = info.argv or {}
  local name = basename(info.executable)
  if name == "ssh" or basename(argv[1]) == "ssh" then
    return M.ssh(argv)
  end
  if name == "mosh-client" or basename(argv[1]) == "mosh-client" then
    local host = (argv[2] or ""):match("^%-# ([^%s]+) |$")
    if host and host:sub(1, 1) ~= "-" then
      return { executable("mosh"), host }
    end
    return nil, "Cannot recover this Mosh connection; use a local split or reconnect"
  end
  for _, child in pairs(info.children or {}) do
    local command, err = M.remote(child)
    if command or err then
      return command, err
    end
  end
end

function M.action(pane, kind, local_host)
  local spawn = { domain = "CurrentPaneDomain" }
  if local_host then
    spawn.domain = { DomainName = "local" }
    spawn.cwd = (options.home or wezterm.home_dir or os.getenv("HOME"))
  else
    local ok, info = pcall(function()
      return pane:get_foreground_process_info()
    end)
    local command, err = M.remote(ok and info or nil)
    if err then
      return nil, err
    end
    if command then
      spawn.domain = { DomainName = "local" }
      spawn.cwd = (options.home or wezterm.home_dir or os.getenv("HOME"))
      spawn.args = command
    end
  end
  if kind == "tab" then
    return wezterm.action.SpawnCommandInNewTab(spawn)
  end
  return wezterm.action.SplitPane({
    direction = kind,
    command = spawn,
    top_level = local_host,
  })
end

return M
