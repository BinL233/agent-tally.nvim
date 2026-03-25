local config = require("agent-tally.config")
local rpc = require("agent-tally.rpc")

local M = {}

local hl_ns = vim.api.nvim_create_namespace("agent_tally_wl_hl")

local state = {
  win = nil,
  buf = nil,
  items = {}, -- { { name = "nvim", enabled = true }, ... }
}

local function is_open()
  return state.win and vim.api.nvim_win_is_valid(state.win)
end

--- Render the watchlist into the buffer.
local function render()
  if not is_open() then
    return
  end

  local lines = {}
  local hls = {}

  -- Header
  table.insert(lines, "  Watchlist")
  table.insert(hls, { 0, 2, 11, "AgentTallySection1" })
  table.insert(lines, "  " .. string.rep("─", 30))
  table.insert(hls, { 1, 0, -1, "AgentTallySection1" })

  for _, item in ipairs(state.items) do
    local check = item.enabled and "[x]" or "[ ]"
    local line = "  " .. check .. " " .. item.name
    table.insert(lines, line)

    -- Highlight the checkbox
    local row = #lines - 1 -- 0-indexed

    if item.enabled then
      table.insert(hls, { row, 2, 5, "AgentTallySection4" })
    else
      table.insert(hls, { row, 2, 5, "AgentTallyHint" })
    end
  end

  table.insert(lines, "")
  table.insert(lines, "  <CR> toggle  a add  d delete  q save & close")
  table.insert(hls, { #lines - 1, 0, -1, "AgentTallyHint" })

  vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

  -- Apply highlights.
  vim.api.nvim_buf_clear_namespace(state.buf, hl_ns, 0, -1)

  for _, h in ipairs(hls) do
    local row, cs, ce, grp = h[1], h[2], h[3], h[4]

    if ce == -1 then
      ce = 9999
    end

    vim.api.nvim_buf_add_highlight(state.buf, hl_ns, grp, row, cs, ce)
  end
end

--- Get the item index for the current cursor line.
local function cursor_item_index()
  if not is_open() then
    return nil
  end

  local row = vim.api.nvim_win_get_cursor(state.win)[1]
  -- Items start at line 3 (1-indexed): header=1, separator=2, first item=3
  local idx = row - 2

  if idx >= 1 and idx <= #state.items then
    return idx
  end

  return nil
end

--- Save the current watchlist to the daemon.
local function save_and_close()
  local enabled = {}

  for _, item in ipairs(state.items) do
    if item.enabled then
      table.insert(enabled, item.name)
    end
  end

  rpc.request(config.current.socket_path, "watchlist-update", { watchlist = enabled }, function(err, _)
    if err then
      vim.schedule(function()
        vim.notify("Failed to update watchlist: " .. err, vim.log.levels.ERROR)
      end)
    end
  end)

  if is_open() then
    vim.api.nvim_win_close(state.win, true)
  end

  state.win = nil
  state.buf = nil
end

--- Open the watchlist configuration window.
function M.open()
  if is_open() then
    vim.api.nvim_set_current_win(state.win)
    return
  end

  -- Fetch current watchlist from daemon.
  rpc.request(config.current.socket_path, "watchlist-get", nil, function(err, result)
    vim.schedule(function()
      if err then
        vim.notify("agent-tally: " .. err, vim.log.levels.ERROR)
        return
      end

      -- Build items: all known defaults + whatever daemon has.
      local defaults = { "claude", "copilot", "cursor", "opencode" }
      local active = {}

      if type(result) == "table" then
        for _, name in ipairs(result) do
          active[name] = true
        end
      end

      -- Merge: defaults + any extra names from daemon.
      local seen = {}
      state.items = {}

      for _, name in ipairs(defaults) do
        table.insert(state.items, { name = name, enabled = active[name] or false })
        seen[name] = true
      end

      -- Add any daemon-side names not in defaults.
      if type(result) == "table" then
        for _, name in ipairs(result) do
          if not seen[name] then
            table.insert(state.items, { name = name, enabled = true })
          end
        end
      end

      -- Create the float.
      local width = 40
      local height = #state.items + 5
      local row = math.floor((vim.o.lines - height) / 2)
      local col = math.floor((vim.o.columns - width) / 2)

      local buf = vim.api.nvim_create_buf(false, true)

      vim.api.nvim_buf_set_option(buf, "bufhidden", "wipe")
      vim.api.nvim_buf_set_option(buf, "buftype", "nofile")

      local win = vim.api.nvim_open_win(buf, true, {
        relative = "editor",
        width = width,
        height = height,
        row = row,
        col = col,
        style = "minimal",
        border = "rounded",
        title = " Watchlist ",
        title_pos = "center",
      })

      vim.api.nvim_win_set_option(win, "cursorline", true)
      vim.api.nvim_win_set_option(win, "winhl", "Normal:Normal,FloatBorder:FloatBorder")

      state.win = win
      state.buf = buf

      local opts = { noremap = true, silent = true, buffer = buf }

      -- Toggle checkbox.
      vim.keymap.set("n", "<CR>", function()
        local idx = cursor_item_index()

        if idx then
          state.items[idx].enabled = not state.items[idx].enabled
          render()
        end
      end, opts)

      vim.keymap.set("n", "<Space>", function()
        local idx = cursor_item_index()

        if idx then
          state.items[idx].enabled = not state.items[idx].enabled
          render()
        end
      end, opts)

      -- Add a new process name.
      vim.keymap.set("n", "a", function()
        vim.ui.input({ prompt = "Process name: " }, function(name)
          if not name or name == "" then
            return
          end

          table.insert(state.items, { name = name, enabled = true })
          render()
        end)
      end, opts)

      -- Delete item under cursor.
      vim.keymap.set("n", "d", function()
        local idx = cursor_item_index()

        if idx then
          table.remove(state.items, idx)
          render()
        end
      end, opts)

      -- Save and close.
      vim.keymap.set("n", "q", save_and_close, opts)
      vim.keymap.set("n", "<Esc>", save_and_close, opts)

      -- Cleanup on close.
      vim.api.nvim_create_autocmd("WinClosed", {
        pattern = tostring(win),
        once = true,
        callback = function()
          state.win = nil
          state.buf = nil
        end,
      })

      render()

      -- Move cursor to first item.
      if #state.items > 0 then
        vim.api.nvim_win_set_cursor(win, { 3, 0 })
      end
    end)
  end)
end

return M
