--- Hook for avante.nvim integration.
--- Automatically intercepts avante's API responses to track token usage.
--- No-op if avante.nvim is not installed.

local M = {}

function M.setup()
  -- Check if avante is available.
  local ok, avante = pcall(require, "avante")

  if not ok then
    return
  end

  local tally = require("agent-tally")

  -- Listen for avante response events.
  -- avante.nvim fires User autocmds that we can hook into.
  vim.api.nvim_create_autocmd("User", {
    pattern = "AvanteResponse",
    callback = function(args)
      local data = args.data or {}

      tally.record({
        process    = "avante",
        file       = vim.api.nvim_buf_get_name(0),
        tokens_in  = data.tokens_input or 0,
        tokens_out = data.tokens_output or 0,
      })
    end,
  })

end

return M
