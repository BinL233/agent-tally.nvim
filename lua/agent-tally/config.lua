local M = {}

M.defaults = {
  -- Path to the agent-tallyd binary.
  daemon_bin = "agent-tallyd",
  -- UNIX socket path (should match the daemon's --socket flag).
  socket_path = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.sock",
  -- PID file path (should match the daemon's --pid flag).
  pid_file = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.pid",
  -- Auto-start the daemon when the plugin loads.
  auto_start = false,
  -- Status line format string. %t = total tokens, %p = process name.
  statusline_format = " [AT: %t tokens]",
  -- Query limits (fetch limits only — the database stores all events).
  query = {
    events_limit = 500,   -- max events loaded into the dashboard
    tools_limit = 50,    -- max tool rows fetched for the By Tool section
  },
  -- UI options.
  ui = {
    width = 0.8,
    height = 0.8,
    border = "rounded",
  },
  -- Keymaps for the dashboard window.
  keymaps = {
    close = { "q", "<Esc>" },
    drill_down = "<CR>",
    back = "<BS>",
    next_entry = "<C-j>",
    prev_entry = "<C-k>",
    grep = "G",
    refresh = "r",
    heatmap = "H",
  },
}

M.current = vim.deepcopy(M.defaults)

function M.apply(opts)
  M.current = vim.tbl_deep_extend("force", M.current, opts or {})
end

return M
