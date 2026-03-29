local config = require("agent-tally.config")
local scan   = require("agent-tally.projconfig.scan")
local fmt    = require("agent-tally.projconfig.format")

local M = {}

local hl_ns = vim.api.nvim_create_namespace("agent_tally_projconfig_hl")

local HINT = "  d delete   \xe2\x86\xb5 open/edit   r refresh   G grep   1-4/Tab switch tab   q close"

local TABS    = { "Claude", "Cursor", "Copilot", "OpenCode" }
local TAB_BAR_HEIGHT = 3  -- tab row + separator + blank line

local state = {
  win        = nil,
  buf        = nil,
  hint_win   = nil,
  hint_buf   = nil,
  all_lines  = nil,
  row_map    = {},   -- 1-based row → { path, deletable }
  cwd        = nil,
  active_tab = 1,
}

local function is_open()
  return state.win and vim.api.nvim_win_is_valid(state.win)
end

local function win_dimensions()
  local ui_conf = config.current.ui
  local editor_w = vim.o.columns
  local editor_h = vim.o.lines - vim.o.cmdheight - 1
  local width  = math.floor(editor_w * ui_conf.width)
  local height = math.floor(editor_h * ui_conf.height)
  local row    = math.floor((editor_h - height) / 2)
  local col    = math.floor((editor_w - width) / 2)
  return { width = width, height = height, row = row, col = col }
end

local function set_hint(text)
  if not state.hint_buf then return end
  vim.api.nvim_buf_set_option(state.hint_buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.hint_buf, 0, -1, false, { text })
  vim.api.nvim_buf_set_option(state.hint_buf, "modifiable", false)
end

local function apply_highlights(hls)
  vim.api.nvim_buf_clear_namespace(state.buf, hl_ns, 0, -1)
  for _, h in ipairs(hls) do
    local row, cs, ce, grp = h[1], h[2], h[3], h[4]
    if ce == -1 then
      local line_len = #(vim.api.nvim_buf_get_lines(state.buf, row, row + 1, false)[1] or "")
      vim.api.nvim_buf_set_extmark(state.buf, hl_ns, row, cs, {
        end_row  = row,
        end_col  = line_len,
        hl_group = grp,
        hl_eol   = true,
      })
    else
      vim.api.nvim_buf_add_highlight(state.buf, hl_ns, grp, row, cs, ce)
    end
  end
end

local function render(lines, hls, row_map)
  state.all_lines = vim.deepcopy(lines)
  state.row_map   = row_map or {}

  vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.api.nvim_buf_set_option(state.buf, "modifiable", false)

  if hls then apply_highlights(hls) end
end

local function open_grep()
  if not is_open() or not state.all_lines then return end

  local win_conf   = vim.api.nvim_win_get_config(state.win)
  local orig_title = win_conf.title

  vim.ui.input({ prompt = "Filter: " }, function(query)
    if not is_open() then return end

    if not query or query == "" then
      vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
      vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, state.all_lines)
      vim.api.nvim_buf_set_option(state.buf, "modifiable", false)
      if orig_title then
        vim.api.nvim_win_set_config(state.win, { title = orig_title })
      end
      return
    end

    -- Always preserve the tab bar (first TAB_BAR_HEIGHT lines).
    local filtered = {}
    for i = 1, TAB_BAR_HEIGHT do
      table.insert(filtered, state.all_lines[i] or "")
    end
    local pat = query:lower()
    for i = TAB_BAR_HEIGHT + 1, #state.all_lines do
      local line = state.all_lines[i]
      if line:lower():find(pat, 1, true) or line == "" then
        table.insert(filtered, line)
      end
    end

    vim.api.nvim_buf_set_option(state.buf, "modifiable", true)
    vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, filtered)
    vim.api.nvim_buf_set_option(state.buf, "modifiable", false)
    vim.api.nvim_win_set_config(state.win, { title = " Filter: " .. query .. " " })
  end)
end

local function load_and_render()
  local data                = scan.scan(state.active_tab, state.cwd)
  local content_lines, content_hls, content_row_map = fmt.dashboard(data)
  local tab_lines, tab_hls = fmt.tab_bar(TABS, state.active_tab)

  local hls = tab_hls
  for _, h in ipairs(content_hls) do
    table.insert(hls, { h[1] + TAB_BAR_HEIGHT, h[2], h[3], h[4] })
  end

  local row_map = {}
  for k, v in pairs(content_row_map) do
    row_map[k + TAB_BAR_HEIGHT] = v
  end

  local lines = {}
  vim.list_extend(lines, tab_lines)
  vim.list_extend(lines, content_lines)

  render(lines, hls, row_map)
end

local function switch_tab(idx)
  if idx < 1 or idx > #TABS then return end
  state.active_tab = idx
  load_and_render()
end

local function set_keymaps(buf)
  local km   = config.current.keymaps
  local opts = { noremap = true, silent = true, buffer = buf }

  -- Close
  local close_keys = type(km.close) == "table" and km.close or { km.close }
  for _, key in ipairs(close_keys) do
    vim.keymap.set("n", key, function() M.close() end, opts)
  end

  -- Refresh
  vim.keymap.set("n", km.refresh, load_and_render, opts)

  -- Grep
  vim.keymap.set("n", km.grep, open_grep, opts)

  -- Tab switching by number
  for i = 1, #TABS do
    vim.keymap.set("n", tostring(i), function() switch_tab(i) end, opts)
  end

  -- Tab / Shift-Tab to cycle
  vim.keymap.set("n", "<Tab>", function()
    switch_tab((state.active_tab % #TABS) + 1)
  end, opts)
  vim.keymap.set("n", "<S-Tab>", function()
    switch_tab(((state.active_tab - 2) % #TABS) + 1)
  end, opts)

  -- Open / edit  (<CR>)
  vim.keymap.set("n", km.drill_down, function()
    if not is_open() then return end
    local row   = vim.api.nvim_win_get_cursor(state.win)[1]
    local entry = state.row_map[row]
    if not entry or not entry.path or entry.path == "" then return end

    local path = entry.path
    M.close()

    local dir = vim.fn.fnamemodify(path, ":h")
    if vim.fn.isdirectory(dir) == 0 then
      vim.fn.mkdir(dir, "p")
    end

    vim.cmd("edit " .. vim.fn.fnameescape(path))
  end, opts)

  -- Delete  (d)
  vim.keymap.set("n", "d", function()
    if not is_open() then return end
    local row   = vim.api.nvim_win_get_cursor(state.win)[1]
    local entry = state.row_map[row]
    if not entry or not entry.path or entry.path == "" then return end
    if not entry.deletable then
      vim.notify("agent-tally: this entry cannot be deleted", vim.log.levels.WARN)
      return
    end

    local path  = entry.path
    local short = vim.fn.fnamemodify(path, ":~")

    local choice = vim.fn.confirm(
      "Delete " .. short .. "?\nThis cannot be undone.",
      "&Yes\n&No",
      2
    )

    if choice ~= 1 then return end

    local ok, err = os.remove(path)
    if ok then
      vim.notify("agent-tally: deleted " .. short, vim.log.levels.INFO)
      load_and_render()
    else
      vim.notify("agent-tally: delete failed: " .. (err or "unknown error"), vim.log.levels.ERROR)
    end
  end, opts)
end

--- Open the config dashboard.
function M.open()
  if is_open() then M.close() end

  local cwd = vim.fn.getcwd()
  state.cwd        = cwd
  state.active_tab = 1

  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_option(buf, "bufhidden", "wipe")
  vim.api.nvim_buf_set_option(buf, "buftype", "nofile")
  vim.api.nvim_buf_set_option(buf, "filetype", "agent-tally")
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { "  Loading..." })
  vim.api.nvim_buf_set_option(buf, "modifiable", false)

  local dim = win_dimensions()

  local win = vim.api.nvim_open_win(buf, true, {
    relative  = "editor",
    width     = dim.width,
    height    = dim.height,
    row       = dim.row,
    col       = dim.col,
    style     = "minimal",
    border    = config.current.ui.border,
    title     = " Agent Tally — Project Config ",
    title_pos = "center",
  })

  vim.api.nvim_win_set_option(win, "cursorline", true)
  vim.api.nvim_win_set_option(win, "winhl", "Normal:Normal,FloatBorder:FloatBorder")

  -- Hint bar
  local hint_buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_option(hint_buf, "bufhidden", "wipe")
  vim.api.nvim_buf_set_option(hint_buf, "buftype", "nofile")
  vim.api.nvim_buf_set_option(hint_buf, "modifiable", false)

  local hint_win = vim.api.nvim_open_win(hint_buf, false, {
    relative  = "editor",
    width     = dim.width - 2,
    height    = 1,
    row       = dim.row + dim.height,
    col       = dim.col + 1,
    style     = "minimal",
    zindex    = 51,
    focusable = false,
  })
  vim.api.nvim_win_set_option(hint_win, "winhl", "Normal:Comment")

  state.win       = win
  state.buf       = buf
  state.hint_win  = hint_win
  state.hint_buf  = hint_buf
  state.all_lines = nil
  state.row_map   = {}

  set_hint(HINT)
  set_keymaps(buf)

  -- Cleanup on window close
  vim.api.nvim_create_autocmd("WinClosed", {
    pattern  = tostring(win),
    once     = true,
    callback = function()
      if state.hint_win and vim.api.nvim_win_is_valid(state.hint_win) then
        vim.api.nvim_win_close(state.hint_win, true)
      end
      state.win       = nil
      state.buf       = nil
      state.hint_win  = nil
      state.hint_buf  = nil
      state.all_lines = nil
      state.row_map   = {}
      state.cwd       = nil
    end,
  })

  -- Load data and render (synchronous — filesystem scan)
  vim.schedule(function()
    if not is_open() then return end
    load_and_render()
  end)
end

--- Close the config dashboard.
function M.close()
  if state.hint_win and vim.api.nvim_win_is_valid(state.hint_win) then
    vim.api.nvim_win_close(state.hint_win, true)
  end
  if is_open() then
    vim.api.nvim_win_close(state.win, true)
  end

  state.win       = nil
  state.buf       = nil
  state.hint_win  = nil
  state.hint_buf  = nil
  state.all_lines = nil
  state.row_map   = {}
  state.cwd       = nil
end

return M
