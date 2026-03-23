local M = {}

M.defaults = {
  -- Path to the agent-tallyd binary.
  daemon_bin = "agent-tallyd",
  -- UNIX socket path (should match the daemon's --socket flag).
  socket_path = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.sock",
  -- Auto-start the daemon when the plugin loads.
  auto_start = false,
  -- Status line format string. %t = total tokens, %p = process name.
  statusline_format = " [AT: %t tokens]",
}

M.current = vim.deepcopy(M.defaults)

function M.apply(opts)
  M.current = vim.tbl_deep_extend("force", M.current, opts or {})
end

return M
