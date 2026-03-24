local M = {}

-- ■ (U+25A0 BLACK SQUARE) for filled cells — compact square with built-in padding,
-- looks visually square in monospace fonts unlike █ (FULL BLOCK).
-- □ (U+25A1 WHITE SQUARE) for empty/zero cells.
local CELL  = "\226\150\160" -- U+25A0  ■
local EMPTY = "\226\150\161" -- U+25A1  □

local DAY_LABELS = { "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun" }
local MONTHS     = { "Jan", "Feb", "Mar", "Apr", "May", "Jun",
                     "Jul", "Aug", "Sep", "Oct", "Nov", "Dec" }

-- 6 intensity levels (index 1 = empty/none, 2-7 = low→high)
local HL_LEVELS = {
  "AgentTallyHeat0",
  "AgentTallyHeat1",
  "AgentTallyHeat2",
  "AgentTallyHeat3",
  "AgentTallyHeat4",
  "AgentTallyHeat5",
  "AgentTallyHeat6",
}

-- Each week column is 3 visual chars wide: 1-char cell + 2-space gap.
-- This lets "Jan" (3 chars) sit flush above its week column with no drift.
local CELL_GAP  = "  "  -- 2 spaces after the cell char
local EMPTY_GAP = "  "  -- same 2 spaces after empty char
local MONTH_GAP = "   " -- 3 spaces for weeks with no month change

--- Return "YYYY-MM-DD" for an os.time value.
local function fmt_date(t)
  return os.date("%Y-%m-%d", t)
end

--- Parse "YYYY-MM-DD" → year, month, day as numbers.
local function parse_date(s)
  local y, m, d = s:match("^(%d%d%d%d)-(%d%d)-(%d%d)$")
  return tonumber(y), tonumber(m), tonumber(d)
end

--- Build a lookup table: day_string → value.
local function build_day_map(day_data, metric)
  local map = {}
  for _, r in ipairs(day_data or {}) do
    local val
    if metric == "Tokens In" then
      val = r.tokens_in or 0
    elseif metric == "Tokens Out" then
      val = r.tokens_out or 0
    else -- "Total"
      val = (r.tokens_in or 0) + (r.tokens_out or 0)
    end
    map[r.day] = val
  end
  return map
end

--- Map a value to intensity bucket 0-6.
--- 0 = no data, 1-6 = quintile buckets of non-zero values.
local function intensity(val, sorted_vals)
  if val == 0 or #sorted_vals == 0 then return 0 end
  local n = #sorted_vals
  local function pct(p) return sorted_vals[math.max(1, math.floor(n * p))] end
  if val <= pct(0.20) then return 1
  elseif val <= pct(0.40) then return 2
  elseif val <= pct(0.60) then return 3
  elseif val <= pct(0.80) then return 4
  elseif val <= pct(0.95) then return 5
  else return 6
  end
end

--- Render a 27-week × 7-day heatmap (~6 months).
--- day_data: array of { day="YYYY-MM-DD", tokens_in, tokens_out }
--- metric:   "Tokens In" | "Tokens Out" | "Total"
--- title:    string shown in the header
---@return string[], table
function M.render(day_data, metric, title)
  local lines = {}
  local hls   = {}

  local indent = "  "

  -- Header
  table.insert(lines, "")
  table.insert(lines, indent .. title)
  table.insert(lines, "")

  -- Build day → value map + sorted non-zero values for quintile calc
  local day_map = build_day_map(day_data, metric)
  local nonzero = {}
  for _, v in pairs(day_map) do
    if v > 0 then table.insert(nonzero, v) end
  end
  table.sort(nonzero)

  -- Compute grid start: Monday of the week 26 weeks ago (~6 months).
  local today_t   = os.time()
  local today_str = fmt_date(today_t)
  local start_t   = today_t - (26 * 7 * 24 * 3600)
  local wday0     = tonumber(os.date("%w", start_t)) -- 0=Sun..6=Sat
  start_t = start_t - ((wday0 == 0 and 6 or wday0 - 1) * 24 * 3600)

  -- Build 27 weeks × 7 days grid; track month of Monday per week for labels.
  local NUM_WEEKS = 27
  local grid      = {}
  local col_month = {}

  for w = 1, NUM_WEEKS do
    grid[w] = {}
    for d = 1, 7 do
      local t  = start_t + ((w - 1) * 7 + (d - 1)) * 24 * 3600
      local ds = fmt_date(t)
      grid[w][d] = ds <= today_str and ds or nil
    end
    if grid[w][1] then
      local _, mo = parse_date(grid[w][1])
      col_month[w] = mo
    end
  end

  -- Month label row.
  -- Each week slot = 3 visual chars: "Jan" for new month, "   " otherwise.
  -- Prefix = "  Mon  " = 7 chars (same as grid row prefix below).
  -- 7-char prefix so month labels line up exactly above cells.
  local LABEL_INDENT = "       " -- 7 chars: matches "  Mon  " prefix
  local month_line   = LABEL_INDENT
  local prev_mo      = -1
  for w = 1, NUM_WEEKS do
    local mo = col_month[w]
    if mo and mo ~= prev_mo then
      month_line = month_line .. MONTHS[mo] -- exactly 3 chars ✓
      prev_mo = mo
    else
      month_line = month_line .. MONTH_GAP  -- exactly 3 chars ✓
    end
  end
  table.insert(lines, month_line)

  -- Grid rows: one per day-of-week (Mon → Sun).
  -- Each cell = char (3 UTF-8 bytes, 1 visual col) + "  " (2 spaces) = 3 visual cols.
  -- Future/out-of-range slots = "   " (3 spaces) = 3 visual cols.
  local CELL_BYTES  = #CELL   -- 3 bytes (UTF-8)
  local EMPTY_BYTES = #EMPTY  -- 3 bytes (UTF-8)

  for d = 1, 7 do
    local label   = DAY_LABELS[d]
    local row_str = indent .. label .. "  "  -- "  Mon  " = 7 chars prefix
    local row_hls = {}
    local col     = #row_str -- byte offset for highlight tracking

    for w = 1, NUM_WEEKS do
      local ds  = grid[w][d]
      local val = ds and (day_map[ds] or 0) or -1

      if val < 0 then
        -- future / out of range: 3-space placeholder
        row_str = row_str .. "   "
        col     = col + 3
      else
        local lv   = intensity(val, nonzero)
        local char = lv == 0 and EMPTY or CELL
        local blen = lv == 0 and EMPTY_BYTES or CELL_BYTES
        local gap  = lv == 0 and EMPTY_GAP or CELL_GAP

        row_str = row_str .. char .. gap
        table.insert(row_hls, { col, col + blen, HL_LEVELS[lv + 1] })
        col = col + blen + #gap
      end
    end

    local row_idx = #lines
    table.insert(lines, row_str)
    for _, h in ipairs(row_hls) do
      table.insert(hls, { row_idx, h[1], h[2], h[3] })
    end
  end

  -- Legend
  table.insert(lines, "")
  local legend_row = #lines
  local lbl_names  = { "very low", "low", "medium", "high", "very high", "max" }
  local legend     = indent .. EMPTY .. " none  "
  for i = 1, 6 do
    legend = legend .. CELL .. " " .. lbl_names[i]
    if i < 6 then legend = legend .. "  " end
  end
  table.insert(lines, legend)

  -- Highlight legend cells
  local lc = #indent
  table.insert(hls, { legend_row, lc, lc + EMPTY_BYTES, HL_LEVELS[1] })
  lc = lc + EMPTY_BYTES + #" none  "
  for i = 1, 6 do
    table.insert(hls, { legend_row, lc, lc + CELL_BYTES, HL_LEVELS[i + 1] })
    lc = lc + CELL_BYTES + 1 + #lbl_names[i]
    if i < 6 then lc = lc + 2 end
  end

  table.insert(lines, "")
  return lines, hls
end

return M
