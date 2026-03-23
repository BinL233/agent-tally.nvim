local M = {}

local rpc = require("agent-tally.rpc")
local config = require("agent-tally.config")

local dashboard_buf = nil
local dashboard_win = nil

local function is_open()
  return dashboard_win
    and vim.api.nvim_win_is_valid(dashboard_win)
    and dashboard_buf
    and vim.api.nvim_buf_is_valid(dashboard_buf)
end

local function close_dashboard()
  if dashboard_win and vim.api.nvim_win_is_valid(dashboard_win) then
    vim.api.nvim_win_close(dashboard_win, true)
  end

  if dashboard_buf and vim.api.nvim_buf_is_valid(dashboard_buf) then
    vim.api.nvim_buf_delete(dashboard_buf, { force = true })
  end

  dashboard_win = nil
  dashboard_buf = nil
end

local function create_float()
  local width = math.min(90, math.floor(vim.o.columns * 0.8))
  local height = math.min(30, math.floor(vim.o.lines * 0.7))
  local row = math.floor((vim.o.lines - height) / 2)
  local col = math.floor((vim.o.columns - width) / 2)

  local buf = vim.api.nvim_create_buf(false, true)

  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    width = width,
    height = height,
    row = row,
    col = col,
    style = "minimal",
    border = "rounded",
    title = " Agent Tally ",
    title_pos = "center",
  })

  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].filetype = "agent-tally"
  vim.wo[win].wrap = false
  vim.wo[win].cursorline = true

  -- Close on q or <Esc>
  vim.keymap.set("n", "q", close_dashboard, { buffer = buf, silent = true })
  vim.keymap.set("n", "<Esc>", close_dashboard, { buffer = buf, silent = true })

  -- Refresh on r
  vim.keymap.set("n", "r", function()
    M.open()
  end, { buffer = buf, silent = true })

  return buf, win
end

local function pad_right(str, len)
  str = tostring(str)

  if #str >= len then
    return str:sub(1, len)
  end

  return str .. string.rep(" ", len - #str)
end

local function pad_left(str, len)
  str = tostring(str)

  if #str >= len then
    return str:sub(1, len)
  end

  return string.rep(" ", len - #str) .. str
end

local function format_number(n)
  local s = tostring(n)
  local result = ""
  local count = 0

  for i = #s, 1, -1 do
    result = s:sub(i, i) .. result
    count = count + 1

    if count % 3 == 0 and i > 1 then
      result = "," .. result
    end
  end

  return result
end

local function render_loading(buf)
  local lines = {
    "",
    "  Loading...",
    "",
  }

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
end

local function render_error(buf, err_msg)
  local lines = {
    "",
    "  Error",
    "  " .. string.rep("-", 40),
    "",
    "  " .. err_msg,
    "",
  }

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
end

local function render_dashboard(buf, status, events)
  local lines = {}

  -- Header
  table.insert(lines, "")
  table.insert(lines, "  Status: " .. (status.status or "unknown"))
  table.insert(lines, "  DB:     " .. (status.db_path or "n/a"))

  -- Watchlist
  local wl = status.watchlist or {}
  table.insert(lines, "  Watch:  " .. table.concat(wl, ", "))
  table.insert(lines, "")

  -- Summary stats
  local total_in = 0
  local total_out = 0
  local by_process = {}

  for _, ev in ipairs(events) do
    total_in = total_in + (ev.tokens_input or 0)
    total_out = total_out + (ev.tokens_output or 0)

    local name = ev.process_name or "(unknown)"
    if not by_process[name] then
      by_process[name] = { input = 0, output = 0, count = 0 }
    end

    by_process[name].input = by_process[name].input + (ev.tokens_input or 0)
    by_process[name].output = by_process[name].output + (ev.tokens_output or 0)
    by_process[name].count = by_process[name].count + 1
  end

  table.insert(lines, "  Tokens  (in: " .. format_number(total_in) .. "  out: " .. format_number(total_out) .. "  total: " .. format_number(total_in + total_out) .. ")")
  table.insert(lines, "  Events: " .. format_number(#events))
  table.insert(lines, "")

  -- Per-process breakdown
  table.insert(lines, "  By Process")
  table.insert(lines, "  " .. string.rep("-", 70))
  table.insert(lines, "  " .. pad_right("Process", 20) .. pad_left("Events", 10) .. pad_left("Tokens In", 14) .. pad_left("Tokens Out", 14) .. pad_left("Total", 14))
  table.insert(lines, "  " .. string.rep("-", 70))

  local sorted = {}
  for name, data in pairs(by_process) do
    table.insert(sorted, { name = name, data = data })
  end

  table.sort(sorted, function(a, b)
    return (a.data.input + a.data.output) > (b.data.input + b.data.output)
  end)

  if #sorted == 0 then
    table.insert(lines, "  " .. pad_right("(no events yet)", 70))
  end

  for _, entry in ipairs(sorted) do
    local d = entry.data
    local total = d.input + d.output

    table.insert(lines, "  "
      .. pad_right(entry.name, 20)
      .. pad_left(format_number(d.count), 10)
      .. pad_left(format_number(d.input), 14)
      .. pad_left(format_number(d.output), 14)
      .. pad_left(format_number(total), 14)
    )
  end

  table.insert(lines, "")

  -- Recent events table
  table.insert(lines, "  Recent Events")
  table.insert(lines, "  " .. string.rep("-", 70))
  table.insert(lines, "  " .. pad_right("Time", 22) .. pad_right("Process", 16) .. pad_right("File", 34))
  table.insert(lines, "  " .. string.rep("-", 70))

  local show_count = math.min(#events, 15)

  if show_count == 0 then
    table.insert(lines, "  " .. pad_right("(no events yet)", 70))
  end

  for i = 1, show_count do
    local ev = events[i]
    local ts = ev.timestamp or ""

    -- Shorten the file path
    local fpath = ev.file_path or ""
    local home = os.getenv("HOME") or ""

    if home ~= "" and fpath:sub(1, #home) == home then
      fpath = "~" .. fpath:sub(#home + 1)
    end

    if #fpath > 32 then
      fpath = "..." .. fpath:sub(-29)
    end

    table.insert(lines, "  "
      .. pad_right(ts, 22)
      .. pad_right(ev.process_name or "", 16)
      .. pad_right(fpath, 34)
    )
  end

  if #events > show_count then
    table.insert(lines, "")
    table.insert(lines, "  ... and " .. (#events - show_count) .. " more events")
  end

  table.insert(lines, "")
  table.insert(lines, "  [q] close    [r] refresh")
  table.insert(lines, "")

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
end

function M.open()
  -- If already open, close and reopen (refresh).
  if is_open() then
    close_dashboard()
  end

  local buf, win = create_float()
  dashboard_buf = buf
  dashboard_win = win

  render_loading(buf)

  -- Fetch status and events in parallel.
  local status_result = nil
  local events_result = nil
  local got_error = false
  local replies = 0

  local function try_render()
    replies = replies + 1

    if replies < 2 then
      return
    end

    vim.schedule(function()
      if not vim.api.nvim_buf_is_valid(buf) then
        return
      end

      if got_error then
        return
      end

      render_dashboard(buf, status_result or {}, events_result or {})
    end)
  end

  local socket = config.current.socket_path

  rpc.request(socket, "status", nil, function(err, result)
    if err then
      got_error = true
      vim.schedule(function()
        if vim.api.nvim_buf_is_valid(buf) then
          render_error(buf, err)
        end
      end)
      return
    end

    status_result = result
    try_render()
  end)

  rpc.request(socket, "query", { Limit = 100 }, function(err, result)
    if err then
      got_error = true
      vim.schedule(function()
        if vim.api.nvim_buf_is_valid(buf) then
          render_error(buf, err)
        end
      end)
      return
    end

    events_result = result or {}
    try_render()
  end)
end

function M.close()
  close_dashboard()
end

return M
