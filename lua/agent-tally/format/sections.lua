local u = require("agent-tally.format.util")

local M = {}

--- Overview: daemon info + token summary.
function M.overview(status, events, cwd)
  events = events or {}
  local lines = {}
  local hls = {}

  local ap = status.active_processes or {}
  local ap_val = #ap > 0 and table.concat(ap, ", ") or "(none detected)"

  u.labeled_line(lines, hls, "Status",    status.status or "unknown",                 "AgentTallySection1")
  u.labeled_line(lines, hls, "Watchlist", table.concat(status.watchlist or {}, ", "), "AgentTallySection1")
  u.labeled_line(lines, hls, "Path",      cwd or table.concat(status.watch_paths or {}, ", "), "AgentTallySection1")
  u.labeled_line(lines, hls, "Active",    ap_val,                                     "AgentTallySection1")

  table.insert(lines, "")

  u.labeled_line(lines, hls, "Events", u.format_number(#events), "AgentTallySection2")

  return lines, hls
end

--- By-process I/O token breakdown table (estimated from file writes).
function M.by_process(events)
  local by = {}

  for _, ev in ipairs(events) do
    local name = ev.process_name or "(unknown)"

    if not by[name] then
      by[name] = { input = 0, output = 0, count = 0 }
    end

    by[name].input  = by[name].input  + (ev.tokens_input  or 0)
    by[name].output = by[name].output + (ev.tokens_output or 0)
    by[name].count  = by[name].count  + 1
  end

  local sorted = {}

  for name, data in pairs(by) do
    table.insert(sorted, { name = name, data = data })
  end

  table.sort(sorted, function(a, b)
    return (a.data.input + a.data.output) > (b.data.input + b.data.output)
  end)

  local rows = { { "Process", "Events", "I/O In", "I/O Out", "I/O Total" } }

  for _, entry in ipairs(sorted) do
    local d = entry.data
    table.insert(rows, {
      entry.name,
      u.format_number(d.count),
      u.format_number(d.input),
      u.format_number(d.output),
      u.format_number(d.input + d.output),
    })
  end

  local lines, hdr, sep = u.align(rows, { "l", "r", "r", "r", "r" })
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection3" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection3" })

  return lines, hls
end

--- By-process actual API token table (from query-tokens).
--- Claude and Copilot show real token counts; other agents show "-".
function M.by_process_tokens(events, token_summaries)
  -- Aggregate event counts per process.
  local by = {}

  for _, ev in ipairs(events) do
    local name = ev.process_name or "(unknown)"
    if not by[name] then
      by[name] = { count = 0 }
    end
    by[name].count = by[name].count + 1
  end

  -- Build a lookup of actual token data keyed by agent name.
  local token_by_agent = {}

  for _, ts in ipairs(token_summaries or {}) do
    if ts.agent then
      token_by_agent[ts.agent] = ts
    end
  end

  -- Also include agents that have token data but no file events.
  for _, ts in ipairs(token_summaries or {}) do
    if ts.agent and not by[ts.agent] then
      by[ts.agent] = { count = 0 }
    end
  end

  local sorted = {}

  for name, data in pairs(by) do
    local actual = token_by_agent[name]
    local sort_val = actual and (actual.tokens_in + actual.tokens_out) or 0
    table.insert(sorted, { name = name, data = data, actual = actual, sort_val = sort_val })
  end

  table.sort(sorted, function(a, b)
    return a.sort_val > b.sort_val
  end)

  local rows = { { "Process", "Events", "API In", "API Out", "API Total" } }

  for _, entry in ipairs(sorted) do
    local t_in, t_out, t_total

    if entry.actual then
      t_in    = u.format_number(entry.actual.tokens_in)
      t_out   = u.format_number(entry.actual.tokens_out)
      t_total = u.format_number(entry.actual.tokens_in + entry.actual.tokens_out)
    else
      t_in    = "-"
      t_out   = "-"
      t_total = "-"
    end

    table.insert(rows, {
      entry.name,
      u.format_number(entry.data.count),
      t_in,
      t_out,
      t_total,
    })
  end

  local lines, hdr, sep = u.align(rows, { "l", "r", "r", "r", "r" })
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection3" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection3" })

  return lines, hls
end

--- Per-file token breakdown table.
function M.by_file(events)
  local by = {}

  for _, ev in ipairs(events) do
    local path = ev.file_path or "(unknown)"

    if not by[path] then
      by[path] = { tokens = 0, count = 0 }
    end

    by[path].tokens = by[path].tokens + (ev.tokens_output or 0)
    by[path].count  = by[path].count + 1
  end

  local sorted = {}

  for path, data in pairs(by) do
    table.insert(sorted, { path = path, data = data })
  end

  table.sort(sorted, function(a, b)
    return a.data.tokens > b.data.tokens
  end)

  local rows = { { "File", "Events", "Tokens Out" } }
  local show = math.min(#sorted, 15)

  for i = 1, show do
    local entry = sorted[i]
    table.insert(rows, {
      u.shorten_path(entry.path, 46),
      u.format_number(entry.data.count),
      u.format_number(entry.data.tokens),
    })
  end

  local lines, hdr, sep = u.align(rows, { "l", "r", "r" })
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection5" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection5" })

  if #sorted > show then
    table.insert(lines, "")
    table.insert(lines, "  → and " .. (#sorted - show) .. " more files — press <CR> to view all")
  end

  return lines, hls
end

--- Full file breakdown table (no row limit).
function M.all_files(events)
  local by = {}

  for _, ev in ipairs(events) do
    local path = ev.file_path or "(unknown)"

    if not by[path] then
      by[path] = { tokens = 0, count = 0 }
    end

    by[path].tokens = by[path].tokens + (ev.tokens_output or 0)
    by[path].count  = by[path].count + 1
  end

  local sorted = {}

  for path, data in pairs(by) do
    table.insert(sorted, { path = path, data = data })
  end

  table.sort(sorted, function(a, b)
    return a.data.tokens > b.data.tokens
  end)

  local rows = { { "File", "Events", "Tokens Out" } }

  for _, entry in ipairs(sorted) do
    table.insert(rows, {
      u.shorten_path(entry.path, 46),
      u.format_number(entry.data.count),
      u.format_number(entry.data.tokens),
    })
  end

  local lines, hdr, sep = u.align(rows, { "l", "r", "r" })
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection5" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection5" })

  return lines, hls
end

--- Recent events table.
function M.recent_events(events, limit)
  limit = limit or 20

  local rows = { { "Time", "PID", "Process", "File" } }
  local count = math.min(#events, limit)

  for i = 1, count do
    local ev = events[i]

    table.insert(rows, {
      ev.timestamp or "",
      tostring(ev.pid or 0),
      ev.process_name or "",
      u.shorten_path(ev.file_path or "", 36),
    })
  end

  local lines, hdr, sep = u.align(rows)
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection4" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection4" })

  if #events > count then
    table.insert(lines, "")
    table.insert(lines, "  → and " .. (#events - count) .. " more events — press <CR> to view all")
  end

  return lines, hls
end

--- Full recent events table (no row limit).
function M.all_events(events)
  local rows = { { "Time", "PID", "Process", "File" } }

  for _, ev in ipairs(events) do
    table.insert(rows, {
      ev.timestamp or "",
      tostring(ev.pid or 0),
      ev.process_name or "",
      u.shorten_path(ev.file_path or "", 36),
    })
  end

  local lines, hdr, sep = u.align(rows)
  local hls = {}

  table.insert(hls, { hdr, 0, -1, "AgentTallySection4" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection4" })

  return lines, hls
end

--- By-tool breakdown table. tool_data is []{ tool_name, count, agent }.
--- Shows max 10 rows; if truncated, appends a "more tools" hint.
function M.by_tool(tool_data)
  tool_data = tool_data or {}

  local lines = {}
  local hls   = {}

  if #tool_data == 0 then
    return lines, hls
  end

  local rows = { { "Tool", "Count", "Agent" } }
  local show = math.min(#tool_data, 10)

  for i = 1, show do
    local s = tool_data[i]
    table.insert(rows, {
      s.tool_name or "(unknown)",
      u.format_number(s.count or 0),
      s.agent or "",
    })
  end

  local out_lines, hdr, sep = u.align(rows, { "l", "r", "l" })
  lines = out_lines

  table.insert(hls, { hdr, 0, -1, "AgentTallySection6" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection6" })

  if #tool_data > show then
    table.insert(lines, "")
    table.insert(lines, "  → and " .. (#tool_data - show) .. " more tools — press <CR> to view all")
  end

  return lines, hls
end

--- Full tool breakdown table (no row limit).
function M.all_tools(tool_data)
  tool_data = tool_data or {}

  local lines = {}
  local hls   = {}

  if #tool_data == 0 then
    return lines, hls
  end

  local rows = { { "Tool", "Count", "Agent" } }

  for _, s in ipairs(tool_data) do
    table.insert(rows, {
      s.tool_name or "(unknown)",
      u.format_number(s.count or 0),
      s.agent or "",
    })
  end

  local out_lines, hdr, sep = u.align(rows, { "l", "r", "l" })
  lines = out_lines

  table.insert(hls, { hdr, 0, -1, "AgentTallySection6" })
  table.insert(hls, { sep, 0, -1, "AgentTallySection6" })

  return lines, hls
end

return M
