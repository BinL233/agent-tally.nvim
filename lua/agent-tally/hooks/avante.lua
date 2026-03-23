--- Hook for avante.nvim integration.
--- Automatically intercepts avante's API responses to track token usage.
--- No-op if avante.nvim is not installed.

local M = {}

-- Per-tab last-seen token usage, used to skip duplicate AvanteViewBufferUpdated fires.
local last_usage = {}

function M.setup()
  -- Check if avante is available.
  local ok, avante = pcall(require, "avante")

  if not ok then
    return
  end

  local tally = require("agent-tally")

  -- AvanteViewBufferUpdated fires after LLM generation completes.
  -- sidebar.chat_history.tokens_usage holds { prompt_tokens, completion_tokens }
  -- for the latest API call. We track last-seen values per tab to avoid
  -- double-counting spurious fires (e.g. codeblock re-parsing).
  vim.api.nvim_create_autocmd("User", {
    pattern = "AvanteViewBufferUpdated",
    callback = function()
      local sidebar = avante.get()
      if not sidebar or not sidebar.chat_history then return end

      local usage = sidebar.chat_history.tokens_usage
      if not usage then return end

      local prompt     = usage.prompt_tokens     or 0
      local completion = usage.completion_tokens or 0
      if prompt == 0 and completion == 0 then return end

      local tab  = vim.api.nvim_get_current_tabpage()
      local prev = last_usage[tab]

      if prev
        and prev.prompt_tokens     == prompt
        and prev.completion_tokens == completion
      then
        return -- same usage as last recorded; spurious fire
      end

      last_usage[tab] = { prompt_tokens = prompt, completion_tokens = completion }

      tally.record({
        process    = "avante",
        file       = vim.api.nvim_buf_get_name(0),
        tokens_in  = prompt,
        tokens_out = completion,
      })
    end,
  })
end

return M
