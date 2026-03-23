local sections = require("agent-tally.format.sections")
local detail = require("agent-tally.format.detail")

local M = {}

--- Format the full dashboard. Returns (lines, hls).
---@param status table
---@param events table[]
---@return string[], table
function M.dashboard(status, events)
  events = events or {}
  local all_lines = {}
  local all_hls   = {}

  local function append(new_lines, new_hls)
    local offset = #all_lines

    for _, l in ipairs(new_lines) do
      table.insert(all_lines, l)
    end

    for _, h in ipairs(new_hls or {}) do
      table.insert(all_hls, { h[1] + offset, h[2], h[3], h[4] })
    end
  end

  append(sections.overview(status, events))
  append({ "" }, {})
  append(sections.by_process(events))
  append({ "" }, {})
  append(sections.by_file(events))
  append({ "" }, {})
  append(sections.recent_events(events))

  local hint_row = #all_lines + 1
  append({ "", "  q close  r refresh  G grep  <CR> details  <BS> back" }, {})
  table.insert(all_hls, { hint_row, 0, -1, "AgentTallyHint" })

  return all_lines, all_hls
end

-- Re-export detail formatters.
M.event_detail = detail.event
M.file_detail  = detail.file

-- Re-export full-table formatters.
M.all_files  = sections.all_files
M.all_events = sections.all_events

return M
