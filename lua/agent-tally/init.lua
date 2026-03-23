local M = {}

local config = require("agent-tally.config")
local rpc = require("agent-tally.rpc")

local daemon_handle = nil

--- Setup the plugin with user options.
---@param opts? table
function M.setup(opts)
  config.apply(opts)

  if config.current.auto_start then
    M.start_daemon()
  end
end

--- Start the sidecar daemon as a background process.
function M.start_daemon()
  if daemon_handle then
    vim.notify("agent-tallyd is already running", vim.log.levels.WARN)
    return
  end

  local bin = config.current.daemon_bin
  local args = { "--socket", config.current.socket_path }

  local handle, pid
  handle, pid = vim.uv.spawn(bin, {
    args = args,
    detached = true,
    stdio = { nil, nil, nil },
  }, function(code, _signal)
    daemon_handle = nil
    if code ~= 0 then
      vim.schedule(function()
        vim.notify("agent-tallyd exited with code " .. code, vim.log.levels.ERROR)
      end)
    end
  end)

  if not handle then
    vim.notify("failed to start agent-tallyd: " .. tostring(pid), vim.log.levels.ERROR)
    return
  end

  daemon_handle = handle
  -- Unref so Neovim can exit without waiting for the daemon.
  handle:unref()
  vim.notify("agent-tallyd started (pid=" .. pid .. ")", vim.log.levels.INFO)
end

--- Stop the sidecar daemon.
function M.stop_daemon()
  if not daemon_handle then
    vim.notify("agent-tallyd is not running", vim.log.levels.WARN)
    return
  end
  daemon_handle:kill("sigterm")
  daemon_handle = nil
  vim.notify("agent-tallyd stopped", vim.log.levels.INFO)
end

--- Query and display daemon status.
function M.status()
  rpc.request(config.current.socket_path, "status", nil, function(err, result)
    vim.schedule(function()
      if err then
        vim.notify("agent-tally: " .. err, vim.log.levels.ERROR)
        return
      end
      vim.notify(vim.inspect(result), vim.log.levels.INFO)
    end)
  end)
end

--- Handle the :AgentTally command.
---@param opts table
function M.command(opts)
  local subcmd = opts.fargs[1] or "dashboard"

  if subcmd == "dashboard" then
    M.status()
  elseif subcmd == "start" then
    M.start_daemon()
  elseif subcmd == "stop" then
    M.stop_daemon()
  else
    vim.notify("Unknown subcommand: " .. subcmd, vim.log.levels.ERROR)
  end
end

return M
