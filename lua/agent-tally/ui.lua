local config = require("agent-tally.config")
local rpc = require("agent-tally.rpc")
local format = require("agent-tally.format")
local heatmap = require("agent-tally.format.heatmap")

local M = {}

-- Namespace for highlight extmarks (cleared and reapplied on every content update).
local hl_ns = vim.api.nvim_create_namespace("agent_tally_hl")

local HINT_MAIN = "  q close  r refresh  G grep  H heatmap  \xe2\x86\xb5 details  \xe2\x8c\xab back"
local HINT_BACK = "  \xe2\x8c\xab back  q close"

-- Singleton state
local state = {
  win = nil,
  buf = nil,
  hint_win = nil,
  hint_buf = nil,
  history = {},
  on_enter = nil,
  all_lines = nil,
  events_cache = nil,
  tools_cache = nil,
  current_hls = nil,
  socket = nil,
  cwd = nil,
}

--- Check if the floating window is currently open.
local function is_open()
  return state.win and vim.api.nvim_win_is_valid(state.win)
end

--- Compute window dimensions from config ratios.
local function win_dimensions()
  local ui_conf = config.current.ui
  local editor_w = vim.o.columns
  local editor_h = vim.o.lines - vim.o.cmdheight - 1

  local width = math.floor(editor_w * ui_conf.width)
  local height = math.floor(editor_h * ui_conf.height)
  local row = math.floor((editor_h - height) / 2)
  local col = math.floor((editor_w - width) / 2)

  return { width = width, height = height, row = row, col = col }
end

--- Get the first data row (after header + separator).
local function first_data_row()
  return 3
end

--- Move cursor to the next entry.
local function move_next()
  if not is_open() then
    return
  end

  local cursor = vim.api.nvim_win_get_cursor(state.win)
  local line_count = vim.api.nvim_buf_line_count(state.buf)
  local next_row = cursor[1] + 1

  if next_row <= line_count then
    vim.api.nvim_win_set_cursor(state.win, { next_row, 0 })
  end
end

--- Move cursor to the previous entry, not going above first data row.
local function move_prev()
  if not is_open() then
    return
  end

  local cursor = vim.api.nvim_win_get_cursor(state.win)
  local prev_row = cursor[1] - 1

  if prev_row >= first_data_row() then
    vim.api.nvim_win_set_cursor(state.win, { prev_row, 0 })
  end
end

--- Filter data lines by query (case-insensitive), preserving header + separator.
---@param query string
local function apply_grep(query)
  if not is_open() or not state.all_lines then
    return
  end

  local lines = {}

  -- Always keep header (line 1) and separator (line 2)
  if #state.all_lines >= 2 then
    table.insert(lines, state.all_lines[1])
    table.insert(lines, state.all_lines[2])
  end

  local pattern = query:lower()

  for i = 3, #state.all_lines do
    if state.all_lines[i]:lower():find(pattern, 1, true) then
      table.insert(lines, state.all_lines[i])
    end
  end

  if #lines == 2 then
    table.insert(lines, "  No matches.")
  end

  vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

  if #lines > 2 then
    vim.api.nvim_win_set_cursor(state.win, { first_data_row(), 0 })
  end
end

--- Open live grep input prompt.
local function open_grep()
  if not is_open() or not state.all_lines then
    return
  end

  local win_conf = vim.api.nvim_win_get_config(state.win)
  local orig_title = win_conf.title

  vim.ui.input({ prompt = "Grep: " }, function(query)
    if not query or query == "" then
      -- Restore full list
      if is_open() then
        vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
        vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, state.all_lines)
        vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

        if orig_title then
          vim.api.nvim_win_set_config(state.win, { title = orig_title })
        end

        if #state.all_lines > 2 then
          vim.api.nvim_win_set_cursor(state.win, { first_data_row(), 0 })
        end
      end
      return
    end

    apply_grep(query)
  end)
end

--- Set buffer keymaps.
local function set_keymaps(buf)
  local km = config.current.keymaps
  local opts = { noremap = true, silent = true, buffer = buf }

  -- Close keymaps
  local close_keys = type(km.close) == "table" and km.close or { km.close }

  for _, key in ipairs(close_keys) do
    vim.keymap.set("n", key, function()
      M.close()
    end, opts)
  end

  -- Navigation
  vim.keymap.set("n", km.next_entry, move_next, opts)
  vim.keymap.set("n", km.prev_entry, move_prev, opts)

  -- Drill-down
  vim.keymap.set("n", km.drill_down, function()
    if state.on_enter then
      local line = vim.api.nvim_get_current_line()
      local cursor = vim.api.nvim_win_get_cursor(state.win)
      state.on_enter(line, cursor[1])
    end
  end, opts)

  -- Back
  vim.keymap.set("n", km.back, function()
    M.go_back()
  end, opts)

  -- Grep
  vim.keymap.set("n", km.grep, open_grep, opts)

  -- Refresh
  vim.keymap.set("n", km.refresh, function()
    M.open()
  end, opts)
end

--- Apply highlight entries to the current buffer.
---@param hls table  list of {row_0idx, col_start, col_end, hl_group}
local function apply_highlights(hls)
  vim.api.nvim_buf_clear_namespace(state.buf, hl_ns, 0, -1)

  for _, h in ipairs(hls) do
    local row, cs, ce, grp = h[1], h[2], h[3], h[4]

    -- col_end of -1 means "rest of line" — use a large number.
    if ce == -1 then
      ce = 9999
    end

    vim.api.nvim_buf_add_highlight(state.buf, hl_ns, grp, row, cs, ce)
  end
end

--- Update the sticky hint bar text.
local function set_hint(text)
  if not state.hint_buf then return end
  vim.api.nvim_buf_set_option(state.hint_buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.hint_buf, 0, -1, false, { text })
  vim.api.nvim_buf_set_option(state.hint_buf, "modifiable", false)
end

--- Replace the content of the floating window.
---@param lines string[]
---@param title string|nil
---@param hls? table  optional highlight entries {row_0idx, col_start, col_end, group}
function M.replace_content(lines, title, hls)
  if not is_open() then
    return
  end

  -- Store full lines for grep filtering
  state.all_lines = vim.deepcopy(lines)

  vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

  if hls then
    apply_highlights(hls)
  end

  if title then
    vim.api.nvim_win_set_config(state.win, { title = " " .. title .. " ", title_pos = "center" })
  end

  -- Move cursor to first data row (after header + separator)
  if #lines > 2 then
    vim.api.nvim_win_set_cursor(state.win, { first_data_row(), 0 })
  end
end

--- Go back to the previous view in the history stack.
function M.go_back()
  if not is_open() or #state.history == 0 then
    return
  end

  local prev = table.remove(state.history)

  vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, prev.lines)
  vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

  state.on_enter = prev.on_enter
  state.all_lines = prev.all_lines

  if prev.hls then
    apply_highlights(prev.hls)
  end

  if prev.title then
    vim.api.nvim_win_set_config(state.win, { title = prev.title })
  end

  -- Restore hint for the view we returned to
  if #state.history == 0 then
    set_hint(HINT_MAIN)
  else
    set_hint(HINT_BACK)
  end
end

--- Push current state to history and show a new view.
local function push_view(lines, title, on_enter, hls)
  if not is_open() then
    return
  end

  -- Save current state to history
  local current_lines = vim.api.nvim_buf_get_lines(state.buf, 0, -1, false)
  local current_title = vim.api.nvim_win_get_config(state.win).title

  table.insert(state.history, {
    lines    = current_lines,
    title    = current_title,
    on_enter = state.on_enter,
    all_lines = state.all_lines,
    hls      = state.current_hls,
  })

  state.on_enter = on_enter
  M.replace_content(lines, title, hls)
  set_hint(HINT_BACK)
end

--- Open the dashboard floating window and fetch data.
function M.open()
  -- If already open, close and reopen (refresh)
  if is_open() then
    M.close()
  end

  -- Create scratch buffer
  local buf = vim.api.nvim_create_buf(false, true)

  vim.api.nvim_buf_set_option(buf, "bufhidden", "wipe")
  vim.api.nvim_buf_set_option(buf, "buftype", "nofile")
  vim.api.nvim_buf_set_option(buf, "filetype", "agent-tally")

  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { "  Loading..." })
  vim.api.nvim_buf_set_option(buf, "modifiable", false)

  -- Compute dimensions
  local dim = win_dimensions()

  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    width = dim.width,
    height = dim.height,
    row = dim.row,
    col = dim.col,
    style = "minimal",
    border = config.current.ui.border,
    title = " Agent Tally ",
    title_pos = "center",
  })

  vim.api.nvim_win_set_option(win, "cursorline", true)
  vim.api.nvim_win_set_option(win, "winhl", "Normal:Normal,FloatBorder:FloatBorder")

  -- Create sticky hint bar: a 1-line float overlaying the bottom of the main float.
  local hint_buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_option(hint_buf, "bufhidden", "wipe")
  vim.api.nvim_buf_set_option(hint_buf, "buftype", "nofile")
  vim.api.nvim_buf_set_option(hint_buf, "modifiable", false)

  local hint_win = vim.api.nvim_open_win(hint_buf, false, {
    relative = "editor",
    width    = dim.width - 2,   -- inside side borders
    height   = 1,
    row      = dim.row + dim.height,     -- last content row (height is content-only; border adds +1 above/below)
    col      = dim.col + 1,     -- inside left border
    style    = "minimal",
    zindex   = 51,              -- above main window (default zindex 50)
    focusable = false,
  })
  vim.api.nvim_win_set_option(hint_win, "winhl", "Normal:Comment")

  state.win = win
  state.buf = buf
  state.hint_win = hint_win
  state.hint_buf = hint_buf
  set_hint(HINT_MAIN)
  state.history = {}
  state.on_enter = nil
  state.all_lines = nil
  state.events_cache = nil
  state.tools_cache = nil

  set_keymaps(buf)

  -- Heatmap keymap (defined here so push_view is in scope)
  local km = config.current.keymaps
  if km.heatmap then
    local opts = { noremap = true, silent = true, buffer = buf }
    vim.keymap.set("n", km.heatmap, function()
      if not state.socket or not state.cwd then return end

      local agent_set = {}
      local agents = { "All Agents" }
      for _, ev in ipairs(state.events_cache or {}) do
        local p = ev.process_name
        if p and p ~= "" and not agent_set[p] then
          agent_set[p] = true
          table.insert(agents, p)
        end
      end

      vim.ui.select({ "This Project", "All Projects" }, { prompt = "Heatmap scope:" }, function(scope)
        if not scope then return end
        vim.ui.select(agents, { prompt = "Agent:" }, function(agent)
          if not agent then return end
          vim.ui.select({ "Tokens In", "Tokens Out", "Total" }, { prompt = "Metric:" }, function(metric)
            if not metric then return end

            local params = {}
            if scope == "This Project" then params.path_prefix = state.cwd end
            if agent ~= "All Agents" then params.process_name = agent end

            local scope_label = scope == "This Project" and state.cwd:match("[^/]+$") or "All Projects"
            local title = "  Heatmap: " .. (agent == "All Agents" and "All Agents" or agent)
                       .. " — " .. metric .. " (" .. scope_label .. ", past 6 months)"

            rpc.request(state.socket, "query-by-day", params, function(_, result)
              vim.schedule(function()
                if not is_open() then return end
                local hm_lines, hm_hls = heatmap.render(result or {}, metric, title)
                push_view(hm_lines, "AgentTally - Heatmap", nil, hm_hls)
              end)
            end)
          end)
        end)
      end)
    end, opts)
  end

  -- Cleanup on window close
  vim.api.nvim_create_autocmd("WinClosed", {
    pattern = tostring(win),
    once = true,
    callback = function()
      if state.hint_win and vim.api.nvim_win_is_valid(state.hint_win) then
        vim.api.nvim_win_close(state.hint_win, true)
      end
      state.win = nil
      state.buf = nil
      state.hint_win = nil
      state.hint_buf = nil
      state.history = {}
      state.on_enter = nil
      state.all_lines = nil
      state.events_cache = nil
      state.tools_cache = nil
      state.current_hls = nil
      state.socket = nil
      state.cwd = nil
    end,
  })

  -- Fetch data
  local socket = config.current.socket_path
  local cwd = vim.fn.getcwd()
  state.socket = socket
  state.cwd = cwd
  local status_result = nil
  local events_result = nil
  local tools_result = nil
  local got_error = false
  local replies = 0

  local function try_render()
    replies = replies + 1

    if replies < 3 then
      return
    end

    vim.schedule(function()
      if not is_open() then
        return
      end

      if got_error then
        return
      end

      -- Defensively ensure both are tables, never nil.
      local status = (type(status_result) == "table") and status_result or {}
      local events = (type(events_result) == "table") and events_result or {}
      local tools = (type(tools_result) == "table") and tools_result or {}

      state.events_cache = events
      state.tools_cache = tools

      -- Build dashboard lines + highlights
      local lines, hls = format.dashboard(status, state.events_cache, cwd, tools)
      state.current_hls = hls

      -- Set on_enter to drill into both recent events and by-file rows.
      state.on_enter = function(line, _row)
        if not state.events_cache or #state.events_cache == 0 then
          return
        end

        local trimmed = vim.trim(line)

        -- Try to match a timestamp (Recent Events table row).
        local ts = trimmed:match("^(%d%d%d%d%-%d%d%-%d%d[T ]%S+)")

        if ts then
          for _, ev in ipairs(state.events_cache) do
            if ev.timestamp and ev.timestamp:find(ts, 1, true) then
              local detail_lines, detail_hls = format.event_detail(ev)
              push_view(detail_lines, "Event Detail", nil, detail_hls)
              return
            end
          end
        end

        -- Try to match a process name (By Process table row).
        -- Process names are simple identifiers with no path separators or timestamps.
        -- Match the first column against known process names in the events cache.
        local first_word = trimmed:match("^(%S+)")

        if first_word and not first_word:match("[/~%.]") and not ts then
          local proc_set = {}

          for _, ev in ipairs(state.events_cache) do
            if ev.process_name and ev.process_name ~= "" then
              proc_set[ev.process_name] = true
            end
          end

          if proc_set[first_word] then
            local detail_lines, detail_hls = format.process_detail(first_word, state.events_cache)
            push_view(detail_lines, first_word .. " Detail", nil, detail_hls)
            return
          end
        end

        -- "→ and N more files" truncation row → show full file table.
        if line:find("more files", 1, true) then
          local full_lines, full_hls = format.all_files(state.events_cache)
          push_view(full_lines, "AgentTally - All Files", nil, full_hls)
          return
        end

        -- "→ and N more events" truncation row → show full events table.
        if line:find("more events", 1, true) then
          local full_lines, full_hls = format.all_events(state.events_cache)
          push_view(full_lines, "AgentTally - All Events", nil, full_hls)
          return
        end

        -- "→ and N more tools" truncation row → show full tool table.
        if line:find("more tools", 1, true) then
          local full_lines, full_hls = format.all_tools(state.tools_cache)
          push_view(full_lines, "AgentTally - All Tools", nil, full_hls)
          return
        end

        -- Try to match a file path (By File table row).
        -- The first column is a (possibly shortened) file path.
        -- Extract the first non-whitespace token from the line.
        local first_col = trimmed:match("^(%S+)")

        if first_col then
          -- Expand ~ back to HOME for matching.
          local home = os.getenv("HOME") or ""
          local expanded = first_col

          if expanded:sub(1, 1) == "~" and home ~= "" then
            expanded = home .. expanded:sub(2)
          end

          -- Also handle "..." prefix from shorten_path.
          local suffix = first_col:match("^%.%.%.(.+)")

          for _, ev in ipairs(state.events_cache) do
            local fp = ev.file_path or ""

            if fp == expanded or (suffix and fp:sub(-#suffix) == suffix) then
              local detail_lines, detail_hls = format.file_detail(fp, state.events_cache)
              push_view(detail_lines, "File Detail", nil, detail_hls)
              return
            end
          end
        end
      end

      M.replace_content(lines, "AgentTally", hls)
      set_hint(HINT_MAIN)
    end)
  end

  -- Ensure the daemon watches the current directory (no-op if already watched).
  rpc.request(socket, "watch-add", { path = cwd }, function() end)

  rpc.request(socket, "status", nil, function(err, result)
    if err then
      got_error = true

      vim.schedule(function()
        if not is_open() then
          return
        end

        local lines = {
          "",
          "  " .. err,
          "",
        }

        vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
        vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
        vim.api.nvim_buf_set_option(state.buf, "modifiable", false)
      end)
      return
    end

    status_result = result
    try_render()
  end)

  rpc.request(socket, "query", { limit = config.current.query.events_limit, path_prefix = cwd }, function(err, result)
    if err then
      -- Still call try_render so the loading screen resolves.
      -- The status error handler (if it fired) already set got_error.
      events_result = {}
      try_render()
      return
    end

    events_result = result or {}
    try_render()
  end)

  rpc.request(socket, "query-tools", { cwd_prefix = cwd, limit = config.current.query.tools_limit }, function(_, result)
    tools_result = result or {}
    try_render()
  end)
end

--- Close the floating window.
function M.close()
  if state.hint_win and vim.api.nvim_win_is_valid(state.hint_win) then
    vim.api.nvim_win_close(state.hint_win, true)
  end
  if is_open() then
    vim.api.nvim_win_close(state.win, true)
  end

  state.win = nil
  state.buf = nil
  state.hint_win = nil
  state.hint_buf = nil
  state.history = {}
  state.on_enter = nil
  state.all_lines = nil
  state.events_cache = nil
  state.tools_cache = nil
  state.current_hls = nil
  state.socket = nil
  state.cwd = nil
end

return M
