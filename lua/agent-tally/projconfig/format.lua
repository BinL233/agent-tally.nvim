local u = require("agent-tally.format.util")

local M = {}

--- Append a section header line with full-row background tint.
local function section_header(lines, hls, title, hl_group)
  if #lines > 0 then table.insert(lines, "") end
  local label = "  " .. title
  local row   = #lines  -- 0-based index of the line we're about to insert
  table.insert(hls, { row, 2, -1, hl_group .. "Header" })
  table.insert(lines, label)
end

--- Build the tab bar (3 lines: tabs row, separator, blank).
--- Returns lines[], hls[] with 0-based row indices.
---@param tabs table   list of tab name strings
---@param active integer  1-based index of the active tab
---@return string[], table
function M.tab_bar(tabs, active)
  local line  = "  "
  local hls   = {}

  for i, name in ipairs(tabs) do
    local label = "  " .. name .. "  "
    local col_start = #line
    line = line .. label
    local col_end = #line
    if i == active then
      table.insert(hls, { 0, col_start, col_end, "AgentTallyTabActive" })
    else
      table.insert(hls, { 0, col_start, col_end, "AgentTallyTabInactive" })
    end
    if i < #tabs then
      table.insert(hls, { 0, col_end, col_end + 1, "AgentTallyTabSep" })
      line = line .. " "
    end
  end

  table.insert(hls, { 1, 0, -1, "AgentTallyTabSep" })

  return { line, string.rep("─", 80), "" }, hls
end

--- Build the full config dashboard.
--- Returns lines, hls, and a row_to_entry map (1-based line index → { path, deletable }).
---@param data table   from scan.scan()
---@return string[], table, table
function M.dashboard(data)
  local lines       = {}
  local hls         = {}
  local row_to_entry = {}

  -- ── Rules ────────────────────────────────────────────────────
  section_header(lines, hls, "[Rules]", "AgentTallySection1")

  local rule_rows = { { "Scope", "File", "Status" } }
  local rule_paths = {}

  for _, r in ipairs(data.rules) do
    table.insert(rule_rows, {
      r.scope,
      u.shorten_path(r.path, 50),
      r.exists and "found" or "not found",
    })
    table.insert(rule_paths, { path = r.path, deletable = r.exists })
  end

  local rule_lines, rule_hdr, rule_sep = u.align(rule_rows, { "l", "l", "l" })
  local base = #lines

  for i, line in ipairs(rule_lines) do
    if i >= 3 then
      if rule_paths[i - 2] then
        row_to_entry[base + i] = rule_paths[i - 2]
      end
    end
    table.insert(lines, line)
  end

  table.insert(hls, { base + rule_hdr, 0, -1, "AgentTallySection1" })
  table.insert(hls, { base + rule_sep, 0, -1, "AgentTallySection1" })

  -- ── Local Overrides ──────────────────────────────────────────
  section_header(lines, hls, "[Local Overrides]", "AgentTallySection2")

  local ov_rows = { { "Scope", "File", "Permissions" } }
  local ov_paths = {}

  for _, o in ipairs(data.overrides) do
    local perm_str
    if not o.exists then
      perm_str = "not found"
    elseif o.rule_count > 0 then
      perm_str = o.rule_count .. " rules"
    else
      perm_str = "0 rules"
    end

    table.insert(ov_rows, {
      o.scope,
      u.shorten_path(o.path, 50),
      perm_str,
    })
    table.insert(ov_paths, { path = o.path, deletable = o.exists })
  end

  local ov_lines, ov_hdr, ov_sep = u.align(ov_rows, { "l", "l", "r" })
  local ov_base = #lines

  for i, line in ipairs(ov_lines) do
    if i >= 3 then
      if ov_paths[i - 2] then
        row_to_entry[ov_base + i] = ov_paths[i - 2]
      end
    end
    table.insert(lines, line)
  end

  table.insert(hls, { ov_base + ov_hdr, 0, -1, "AgentTallySection2" })
  table.insert(hls, { ov_base + ov_sep, 0, -1, "AgentTallySection2" })

  -- ── Memory ───────────────────────────────────────────────────
  section_header(lines, hls, "[Memory]", "AgentTallySection3")

  if #data.memory == 0 then
    table.insert(lines, "   (no memory entries for this project)")
  else
    local mem_rows = { { "Name", "Type", "Description" } }
    local mem_paths = {}

    for _, m in ipairs(data.memory) do
      local desc = m.description
      if #desc > 48 then desc = desc:sub(1, 45) .. "..." end
      table.insert(mem_rows, { m.name, m.type, desc })
      table.insert(mem_paths, { path = m.path, deletable = true })
    end

    local mem_lines, mem_hdr, mem_sep = u.align(mem_rows, { "l", "l", "l" })
    local mem_base = #lines

    for i, line in ipairs(mem_lines) do
      if i >= 3 then
        if mem_paths[i - 2] then
          row_to_entry[mem_base + i] = mem_paths[i - 2]
        end
      end
      table.insert(lines, line)
    end

    table.insert(hls, { mem_base + mem_hdr, 0, -1, "AgentTallySection3" })
    table.insert(hls, { mem_base + mem_sep, 0, -1, "AgentTallySection3" })
  end

  -- ── Skills ───────────────────────────────────────────────────
  section_header(lines, hls, "[Skills]", "AgentTallySection4")

  if #data.skills == 0 then
    table.insert(lines, "   (no skills installed)")
  else
    local sk_rows = { { "Name", "Path" } }
    local sk_paths = {}

    for _, s in ipairs(data.skills) do
      table.insert(sk_rows, { s.name, u.shorten_path(s.path, 52) })
      table.insert(sk_paths, { path = s.path, deletable = false })
    end

    local sk_lines, sk_hdr, sk_sep = u.align(sk_rows, { "l", "l" })
    local sk_base = #lines

    for i, line in ipairs(sk_lines) do
      if i >= 3 then
        if sk_paths[i - 2] then
          row_to_entry[sk_base + i] = sk_paths[i - 2]
        end
      end
      table.insert(lines, line)
    end

    table.insert(hls, { sk_base + sk_hdr, 0, -1, "AgentTallySection4" })
    table.insert(hls, { sk_base + sk_sep, 0, -1, "AgentTallySection4" })
  end

  -- ── Installed Plugins ────────────────────────────────────────
  section_header(lines, hls, "[Installed Plugins]", "AgentTallySection5")

  if #data.plugins == 0 then
    table.insert(lines, "   (no plugins installed)")
  else
    local pl_rows = { { "Plugin", "Scope", "Version" } }
    local pl_paths = {}

    for _, p in ipairs(data.plugins) do
      table.insert(pl_rows, {
        u.shorten_path(p.name, 40),
        p.scope,
        p.version,
      })
      table.insert(pl_paths, { path = p.path, deletable = false })
    end

    local pl_lines, pl_hdr, pl_sep = u.align(pl_rows, { "l", "l", "l" })
    local pl_base = #lines

    for i, line in ipairs(pl_lines) do
      if i >= 3 then
        if pl_paths[i - 2] then
          row_to_entry[pl_base + i] = pl_paths[i - 2]
        end
      end
      table.insert(lines, line)
    end

    table.insert(hls, { pl_base + pl_hdr, 0, -1, "AgentTallySection5" })
    table.insert(hls, { pl_base + pl_sep, 0, -1, "AgentTallySection5" })
  end

  table.insert(lines, "")

  return lines, hls, row_to_entry
end

return M
