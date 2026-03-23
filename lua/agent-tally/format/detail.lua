local u = require("agent-tally.format.util")

local M = {}

--- Format a single event detail view.
---@param event table
---@return string[], table
function M.event(event)
  local lines = {}
  local hls   = {}

  local function row(label, value)
    u.labeled_line(lines, hls, label, value, "AgentTallySection2")
  end

  row("Timestamp",  event.timestamp or "")
  row("PID",        tostring(event.pid or 0))
  row("Process",    event.process_name or "")
  row("File",       event.file_path or "")
  row("Tokens In",  u.format_number(event.tokens_input or 0))
  row("Tokens Out", u.format_number(event.tokens_output or 0))

  table.insert(lines, "")
  table.insert(hls, { #lines, 0, -1, "AgentTallyHint" })
  table.insert(lines, "  <BS> back  q close")

  return lines, hls
end

--- Format a file detail view — stats + all events for a specific file.
---@param file_path string
---@param events table[]
---@return string[], table
function M.file(file_path, events)
  local lines = {}
  local hls   = {}

  table.insert(hls, { #lines, 2, 6, "AgentTallySection5" })
  table.insert(lines, "  File  " .. file_path)
  table.insert(lines, "")

  local total_in, total_out, count = 0, 0, 0

  for _, ev in ipairs(events) do
    if ev.file_path == file_path then
      total_in  = total_in  + (ev.tokens_input  or 0)
      total_out = total_out + (ev.tokens_output or 0)
      count     = count + 1
    end
  end

  u.labeled_line(lines, hls, "Events",     u.format_number(count),                "AgentTallySection2")
  u.labeled_line(lines, hls, "Tokens In",  u.format_number(total_in),             "AgentTallySection2")
  u.labeled_line(lines, hls, "Tokens Out", u.format_number(total_out),            "AgentTallySection2")
  u.labeled_line(lines, hls, "Total",      u.format_number(total_in + total_out), "AgentTallySection2")

  table.insert(lines, "")

  local rows = { { "Time", "PID", "Process", "Tokens Out" } }

  for _, ev in ipairs(events) do
    if ev.file_path == file_path then
      table.insert(rows, {
        ev.timestamp or "",
        tostring(ev.pid or 0),
        ev.process_name or "",
        u.format_number(ev.tokens_output or 0),
      })
    end
  end

  local tbl_lines, hdr, sep = u.align(rows, { "l", "r", "l", "r" })
  local offset = #lines

  for _, l in ipairs(tbl_lines) do
    table.insert(lines, l)
  end

  table.insert(hls, { hdr + offset, 0, -1, "AgentTallySection4" })
  table.insert(hls, { sep + offset, 0, -1, "AgentTallySection4" })

  table.insert(lines, "")
  table.insert(hls, { #lines, 0, -1, "AgentTallyHint" })
  table.insert(lines, "  <BS> back  q close")

  return lines, hls
end

return M
