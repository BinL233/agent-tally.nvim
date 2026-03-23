local M = {}

local config = require("agent-tally.config")
local rpc = require("agent-tally.rpc")
local ui = require("agent-tally.ui")

local daemon_handle = nil

--- Setup the plugin with user options.
---@param opts? table
function M.setup(opts)
  config.apply(opts)

  if config.current.auto_start then
    M.start_daemon()
  end

  -- Load AI plugin hooks (no-op if plugins aren't installed).
  require("agent-tally.hooks.avante").setup()

  -- On exit, remove the current directory from the daemon's watch list.
  vim.api.nvim_create_autocmd("VimLeavePre", {
    once = true,
    callback = function()
      rpc.request(config.current.socket_path, "watch-remove", { path = vim.fn.getcwd() }, function() end)
    end,
  })
end

--- Probe the socket to check if a daemon is already answering, then call cb(alive).
local function probe_socket(cb)
  local uv = vim.uv or vim.loop
  local pipe = uv.new_pipe(false)

  pipe:connect(config.current.socket_path, function(err)
    pipe:close()
    cb(err == nil)
  end)
end

--- Spawn the daemon binary. Calls cb(ok, err_msg) when spawn completes or fails.
local function spawn_daemon(cb)
  local bin = config.current.daemon_bin
  local args = {
    "--socket", config.current.socket_path,
    "--pid",    config.current.pid_file,
  }

  local handle, pid
  handle, pid = vim.uv.spawn(bin, {
    args     = args,
    detached = true,
    stdio    = { nil, nil, nil },
  }, function(code, _signal)
    daemon_handle = nil

    if code ~= 0 then
      vim.schedule(function()
        vim.notify(
          "agent-tallyd exited with code " .. code
            .. " (another instance may be running — check pid file: "
            .. config.current.pid_file .. ")",
          vim.log.levels.ERROR
        )
      end)
    end
  end)

  if not handle then
    if cb then
      cb(false, tostring(pid))
    end
    return
  end

  daemon_handle = handle
  handle:unref()

  if cb then
    cb(true, pid)
  end
end

--- Poll the socket until it answers, then call cb(). Gives up after max_attempts.
local function wait_for_socket(cb, attempt, max_attempts)
  attempt      = attempt or 1
  max_attempts = max_attempts or 15  -- ~1.5 seconds total

  if attempt > max_attempts then
    vim.schedule(function()
      vim.notify("agent-tallyd did not start in time", vim.log.levels.ERROR)
    end)
    return
  end

  probe_socket(function(alive)
    if alive then
      vim.schedule(cb)
      return
    end

    local timer = vim.uv.new_timer()
    timer:start(100, 0, function()
      timer:close()
      wait_for_socket(cb, attempt + 1, max_attempts)
    end)
  end)
end

--- Start the sidecar daemon as a background process.
function M.start_daemon()
  -- First check the in-process handle (same Neovim session).
  if daemon_handle then
    vim.notify("agent-tallyd is already running", vim.log.levels.WARN)
    return
  end

  -- Then probe the socket — catches externally started or previous-session daemons.
  probe_socket(function(alive)
    if alive then
      vim.schedule(function()
        vim.notify("agent-tallyd is already running (detected via socket)", vim.log.levels.WARN)
      end)
      return
    end

    spawn_daemon(function(ok, pid_or_err)
      if not ok then
        vim.schedule(function()
          vim.notify("failed to start agent-tallyd: " .. pid_or_err, vim.log.levels.ERROR)
        end)
        return
      end

      vim.schedule(function()
        vim.notify("agent-tallyd started (pid=" .. pid_or_err .. ")", vim.log.levels.INFO)
      end)
    end)
  end)
end

--- Stop the sidecar daemon.
function M.stop_daemon()
  -- Try handle first (same session).
  if daemon_handle then
    daemon_handle:kill("sigterm")
    daemon_handle = nil
    vim.notify("agent-tallyd stopped", vim.log.levels.INFO)
    return
  end

  -- Fallback: read the PID file and signal the process directly.
  local pid_file = config.current.pid_file
  local f = io.open(pid_file, "r")

  if not f then
    vim.notify("agent-tallyd is not running (no pid file found)", vim.log.levels.WARN)
    return
  end

  local pid = tonumber(f:read("*l"))
  f:close()

  if not pid then
    vim.notify("agent-tallyd pid file is invalid", vim.log.levels.WARN)
    return
  end

  local ok, err = pcall(function()
    vim.uv.kill(pid, "sigterm")
  end)

  if ok then
    vim.notify("agent-tallyd stopped (pid=" .. pid .. ")", vim.log.levels.INFO)
    os.remove(pid_file)
  else
    vim.notify("failed to stop agent-tallyd: " .. tostring(err), vim.log.levels.ERROR)
  end
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

--- Record an AI token event directly (for Neovim AI plugin integrations).
--- This bypasses the file watcher — use it when an AI plugin reports its own usage.
---@param opts table { process: string, file?: string, tokens_in?: number, tokens_out?: number }
function M.record(opts)
  if not opts or not opts.process then
    vim.notify("agent-tally.record: 'process' is required", vim.log.levels.ERROR)
    return
  end

  local event = {
    process_name  = opts.process,
    file_path     = opts.file or "",
    tokens_input  = opts.tokens_in or 0,
    tokens_output = opts.tokens_out or 0,
    pid           = opts.pid or 0,
  }

  rpc.request(config.current.socket_path, "record-event", event, function(err, _)
    if err then
      vim.schedule(function()
        vim.notify("agent-tally.record: " .. err, vim.log.levels.ERROR)
      end)
    end
  end)
end

--- Clear events for the current working directory (with confirmation).
function M.clean()
  local cwd = vim.fn.getcwd()
  local choice = vim.fn.confirm(
    "Clear agent-tally events for:\n" .. cwd .. "\nThis cannot be undone.",
    "&Yes\n&No",
    2
  )

  if choice ~= 1 then
    return
  end

  rpc.request(config.current.socket_path, "clear-path", { path = cwd }, function(err, _)
    vim.schedule(function()
      if err then
        vim.notify("agent-tally: " .. err, vim.log.levels.ERROR)
        return
      end

      vim.notify("agent-tally: events cleared for " .. cwd, vim.log.levels.INFO)
    end)
  end)
end

--- Clear all recorded events from the database (with confirmation).
function M.clean_all()
  local choice = vim.fn.confirm(
    "Clear ALL agent-tally events from the database?\nThis cannot be undone.",
    "&Yes\n&No",
    2
  )

  if choice ~= 1 then
    return
  end

  rpc.request(config.current.socket_path, "clear", nil, function(err, _)
    vim.schedule(function()
      if err then
        vim.notify("agent-tally: " .. err, vim.log.levels.ERROR)
        return
      end

      vim.notify("agent-tally: all events cleared", vim.log.levels.INFO)
    end)
  end)
end

--- Clear all recorded events from the database (with confirmation).
--- Kept for backward compatibility with :AgentTally clear subcommand.
function M.clear()
  M.clean_all()
end

--- Open the tally dashboard, auto-starting the daemon if it is not running.
function M.dashboard()
  probe_socket(function(alive)
    if alive then
      vim.schedule(function() ui.open() end)
      return
    end

    -- Daemon not running — start it silently, then open once the socket is ready.
    if daemon_handle then
      -- Already spawned this session but socket not up yet; just wait.
      wait_for_socket(ui.open)
      return
    end

    spawn_daemon(function(ok, pid_or_err)
      if not ok then
        vim.schedule(function()
          vim.notify("agent-tally: failed to start daemon: " .. pid_or_err, vim.log.levels.ERROR)
        end)
        return
      end

      vim.schedule(function()
        vim.notify("agent-tallyd started (pid=" .. pid_or_err .. ")", vim.log.levels.INFO)
      end)

      wait_for_socket(ui.open)
    end)
  end)
end

--- Handle the :AgentTally command.
---@param opts table
function M.command(opts)
  local subcmd = opts.fargs[1] or "dashboard"

  if subcmd == "dashboard" then
    M.dashboard()
  elseif subcmd == "start" then
    M.start_daemon()
  elseif subcmd == "stop" then
    M.stop_daemon()
  elseif subcmd == "watchlist" then
    require("agent-tally.watchlist").open()
  elseif subcmd == "clear" then
    M.clear()
  else
    vim.notify("Unknown subcommand: " .. subcmd, vim.log.levels.ERROR)
  end
end

return M
