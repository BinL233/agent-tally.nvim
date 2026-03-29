local M = {}

--- Right-pad a string to a given width.
function M.pad(s, width)
  s = tostring(s or "")

  if #s >= width then
    return s
  end

  return s .. string.rep(" ", width - #s)
end

--- Right-align a string to a given width.
function M.pad_left(s, width)
  s = tostring(s or "")

  if #s >= width then
    return s
  end

  return string.rep(" ", width - #s) .. s
end

--- Format a number with comma separators.
function M.format_number(n)
  local s = tostring(n or 0)
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

--- Shorten a file path for display.
function M.shorten_path(fpath, max_len)
  local home = os.getenv("HOME") or ""

  if home ~= "" and fpath:sub(1, #home) == home then
    fpath = "~" .. fpath:sub(#home + 1)
  end

  if #fpath > max_len then
    fpath = "..." .. fpath:sub(-(max_len - 3))
  end

  return fpath
end

--- Compute per-column widths from one or more row sets.
---@param ... string[][]  one or more row arrays
---@return number[]
function M.compute_widths(...)
  local widths = {}
  for _, rows in ipairs({ ... }) do
    for _, row in ipairs(rows) do
      for i, cell in ipairs(row) do
        local len = #tostring(cell)
        widths[i] = math.max(widths[i] or 0, len)
      end
    end
  end
  return widths
end

--- Build aligned lines from rows with header + separator.
--- First row is treated as the header. Supports per-column alignment.
---@param rows string[][]
---@param alignments? string[] "l" for left (default), "r" for right per column
---@param min_widths? number[] minimum width per column (for shared alignment across tables)
---@return string[], number, number  (lines, header_row_0idx, sep_row_0idx)
function M.align(rows, alignments, min_widths)
  if #rows == 0 then
    return { "  No data." }, 0, 0
  end

  local widths = M.compute_widths(rows)

  if min_widths then
    for i, w in ipairs(min_widths) do
      widths[i] = math.max(widths[i] or 0, w)
    end
  end

  local lines = {}

  for ri, row in ipairs(rows) do
    local parts = {}

    for i, cell in ipairs(row) do
      local a = alignments and alignments[i] or "l"

      if a == "r" then
        table.insert(parts, M.pad_left(cell, widths[i]))
      else
        table.insert(parts, M.pad(cell, widths[i]))
      end
    end

    table.insert(lines, "   " .. table.concat(parts, "   "))

    if ri == 1 then
      local sep_parts = {}

      for i = 1, #widths do
        table.insert(sep_parts, string.rep("─", widths[i]))
      end

      table.insert(lines, "   " .. table.concat(sep_parts, "───"))
    end
  end

  return lines, 0, 1
end

--- Append a labeled key-value line with a highlight on the label.
function M.labeled_line(lines, hls, label, value, hl_group)
  table.insert(hls, { #lines, 2, 2 + #label, hl_group })
  table.insert(lines, "  " .. M.pad(label, 12) .. value)
end

return M
