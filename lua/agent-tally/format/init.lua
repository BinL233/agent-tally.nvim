local sections = require("agent-tally.format.sections")
local detail = require("agent-tally.format.detail")

local M = {}

--- Format the full dashboard. Returns (lines, hls).
---@param status table
---@param events table[]
---@param cwd string
---@param tools table[]
---@param tokens table[] token summaries from query-tokens (agent, tokens_in, tokens_out)
---@return string[], table
function M.dashboard(status, events, cwd, tools, tokens)
  events = events or {}
  tokens = tokens or {}
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

  append(sections.overview(status, events, cwd, tokens))
  append({ "" }, {})

  local api_lines, api_hls, io_lines, io_hls = sections.process_tables(events, tokens)
  append(api_lines, api_hls)
  append({ "" }, {})
  append(io_lines, io_hls)
  append({ "" }, {})

  append(sections.by_file(events))
  append({ "" }, {})

  local tool_lines, tool_hls = sections.by_tool(tools)
  if #tool_lines > 0 then
    append(tool_lines, tool_hls)
    append({ "" }, {})
  end

  append(sections.recent_events(events))

  append({ "" }, {})

  return all_lines, all_hls
end

-- Re-export detail formatters.
M.event_detail   = detail.event
M.file_detail    = detail.file
M.process_detail = detail.process

-- Re-export full-table formatters.
M.all_files   = sections.all_files
M.all_events  = sections.all_events
M.all_tools  = sections.all_tools

return M
