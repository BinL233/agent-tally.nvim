local M = {}

local home = vim.fn.expand("~")

--- Check if a file exists on disk.
local function exists(path)
  return vim.fn.filereadable(path) == 1
end

--- Encode a filesystem path the same way Claude Code does:
--- replace all slashes with dashes (leading slash becomes leading dash).
local function encode_path(path)
  return path:gsub("/", "-")
end

--- Read and JSON-decode a file. Returns decoded table or nil.
local function read_json(path)
  if not exists(path) then return nil end
  local f = io.open(path, "r")
  if not f then return nil end
  local raw = f:read("*a")
  f:close()
  local ok, decoded = pcall(vim.fn.json_decode, raw)
  return ok and type(decoded) == "table" and decoded or nil
end

--- Count permission entries in a Claude settings JSON file.
--- Returns 0 if the file doesn't exist or can't be parsed.
local function count_permissions(path)
  local decoded = read_json(path)
  if not decoded then return 0 end
  local allow = decoded.permissions and decoded.permissions.allow
  return type(allow) == "table" and #allow or 0
end

--- Parse YAML frontmatter from a markdown file.
--- Returns a table of key/value pairs, or {}.
local function parse_frontmatter(path)
  local f = io.open(path, "r")
  if not f then return {} end

  local lines = {}
  local in_fm = false
  local found_open = false

  for line in f:lines() do
    if not found_open then
      if line:match("^%-%-%-") then
        found_open = true
        in_fm = true
      else
        break
      end
    elseif in_fm then
      if line:match("^%-%-%-") then
        break
      end
      table.insert(lines, line)
    end
  end

  f:close()

  local fm = {}
  for _, line in ipairs(lines) do
    local key, val = line:match("^(%w+):%s*(.+)")
    if key then
      fm[key] = val
    end
  end

  return fm
end

--- Glob SKILL.md files under a directory and parse their frontmatter.
local function scan_skill_dir(dir)
  local skills = {}
  local entries = vim.fn.glob(dir .. "/*/SKILL.md", false, true)
  for _, fpath in ipairs(entries) do
    local fm   = parse_frontmatter(fpath)
    local name = vim.fn.fnamemodify(vim.fn.fnamemodify(fpath, ":h"), ":t")
    table.insert(skills, {
      name        = fm.name or name,
      description = fm.description or "",
      path        = fpath,
    })
  end
  return skills
end

--- Empty data scaffold.
local function empty_data()
  return { rules = {}, overrides = {}, memory = {}, skills = {}, plugins = {} }
end

--- Scan all Claude Code config files for a project.
---@param cwd string  current working directory
---@return table  { rules, overrides, memory, skills, plugins }
function M.scan_claude(cwd)
  local data = empty_data()

  -- ── Rules ──────────────────────────────────────────────────
  local rule_paths = {
    { scope = "Global",  path = home .. "/.claude/CLAUDE.md" },
    { scope = "Project", path = cwd .. "/CLAUDE.md" },
    { scope = "Project", path = cwd .. "/.claude/CLAUDE.md" },
  }

  for _, r in ipairs(rule_paths) do
    table.insert(data.rules, {
      scope  = r.scope,
      path   = r.path,
      exists = exists(r.path),
    })
  end

  -- ── Local Overrides ─────────────────────────────────────────
  local override_paths = {
    { scope = "Global",  path = home .. "/.claude/settings.json" },
    { scope = "Project", path = cwd .. "/.claude/settings.json" },
    { scope = "Project", path = cwd .. "/.claude/settings.local.json" },
  }

  for _, o in ipairs(override_paths) do
    table.insert(data.overrides, {
      scope      = o.scope,
      path       = o.path,
      exists     = exists(o.path),
      rule_count = count_permissions(o.path),
    })
  end

  -- ── Memory ──────────────────────────────────────────────────
  local encoded   = encode_path(cwd)
  local mem_dir   = home .. "/.claude/projects/" .. encoded .. "/memory"
  local mem_files = vim.fn.glob(mem_dir .. "/*.md", false, true)

  for _, fpath in ipairs(mem_files) do
    local fname = vim.fn.fnamemodify(fpath, ":t")
    if fname ~= "MEMORY.md" then
      local fm = parse_frontmatter(fpath)
      table.insert(data.memory, {
        name        = fm.name or fname:gsub("%.md$", ""),
        type        = fm.type or "",
        description = fm.description or "",
        path        = fpath,
      })
    end
  end

  -- ── Skills ──────────────────────────────────────────────────
  local skill_entries = vim.fn.glob(home .. "/.claude/skills/*", false, true)

  for _, spath in ipairs(skill_entries) do
    table.insert(data.skills, {
      name = vim.fn.fnamemodify(spath, ":t"),
      path = vim.fn.resolve(spath),
    })
  end

  -- ── Installed Plugins ────────────────────────────────────────
  local decoded = read_json(home .. "/.claude/plugins/installed_plugins.json")
  if decoded then
    -- version-2 format wraps entries under a "plugins" key
    local plugin_map = (decoded.plugins and type(decoded.plugins) == "table")
      and decoded.plugins or decoded
    for id, entries in pairs(plugin_map) do
      -- each entry is an array of installs; show the first (most recent) one
      local p = type(entries) == "table" and entries[1] or nil
      if p and type(p) == "table" then
        table.insert(data.plugins, {
          name    = id,
          scope   = p.scope or "",
          version = p.version or "",
          path    = p.installPath or "",
        })
      end
    end
  end

  return data
end

--- Scan Cursor config files for a project.
---@param cwd string
---@return table  { rules, overrides, memory, skills, plugins }
function M.scan_cursor(cwd)
  local data = empty_data()

  -- ── Rules ──────────────────────────────────────────────────
  local rule_paths = {
    { scope = "Project", path = cwd .. "/.cursorrules" },
    { scope = "Project", path = cwd .. "/.cursor/rules" },
    { scope = "Global",  path = home .. "/.cursor/rules" },
  }
  for _, r in ipairs(rule_paths) do
    table.insert(data.rules, {
      scope  = r.scope,
      path   = r.path,
      exists = exists(r.path),
    })
  end

  -- ── Local Overrides ─────────────────────────────────────────
  local cli_config = home .. "/.cursor/cli-config.json"
  local cli_exists = exists(cli_config)
  local shell_count = 0
  if cli_exists then
    local decoded = read_json(cli_config)
    if decoded and type(decoded.shellAllowList) == "table" then
      shell_count = vim.tbl_count(decoded.shellAllowList)
    end
  end
  table.insert(data.overrides, {
    scope      = "Global",
    path       = cli_config,
    exists     = cli_exists,
    rule_count = shell_count,
  })

  -- ── Skills ──────────────────────────────────────────────────
  data.skills = scan_skill_dir(home .. "/.cursor/skills-cursor")

  return data
end

--- Scan GitHub Copilot config files for a project.
---@param cwd string
---@return table  { rules, overrides, memory, skills, plugins }
function M.scan_copilot(cwd)
  local data = empty_data()

  -- ── Rules ──────────────────────────────────────────────────
  local rule_paths = {
    { scope = "Project", path = cwd .. "/.github/copilot-instructions.md" },
    { scope = "Global",  path = home .. "/.copilot/instructions.md" },
  }
  for _, r in ipairs(rule_paths) do
    table.insert(data.rules, {
      scope  = r.scope,
      path   = r.path,
      exists = exists(r.path),
    })
  end

  -- ── Local Overrides ─────────────────────────────────────────
  local cfg = home .. "/.copilot/config.json"
  local cfg_exists = exists(cfg)
  local folder_count = 0
  if cfg_exists then
    local decoded = read_json(cfg)
    if decoded and type(decoded.trusted_folders) == "table" then
      folder_count = #decoded.trusted_folders
    end
  end
  table.insert(data.overrides, {
    scope      = "Global",
    path       = cfg,
    exists     = cfg_exists,
    rule_count = folder_count,
  })

  -- ── Skills ──────────────────────────────────────────────────
  data.skills = scan_skill_dir(home .. "/.copilot/skills")

  -- ── Installed Plugins (Copilot extension versions) ──────────
  local decoded = read_json(home .. "/.config/github-copilot/versions.json")
  if decoded then
    for name, info in pairs(decoded) do
      if type(info) == "table" then
        table.insert(data.plugins, {
          name    = name,
          scope   = "Global",
          version = info.version or "",
          path    = info.path or "",
        })
      elseif type(info) == "string" then
        table.insert(data.plugins, {
          name    = name,
          scope   = "Global",
          version = info,
          path    = "",
        })
      end
    end
  end

  return data
end

--- Scan OpenCode config files for a project.
---@param cwd string
---@return table  { rules, overrides, memory, skills, plugins }
function M.scan_opencode(cwd)
  local data = empty_data()

  -- ── Rules (project config files) ───────────────────────────
  local rule_paths = {
    { scope = "Project", path = cwd .. "/opencode.json" },
    { scope = "Project", path = cwd .. "/.opencode.json" },
    { scope = "Global",  path = home .. "/.config/opencode/config.json" },
  }
  for _, r in ipairs(rule_paths) do
    table.insert(data.rules, {
      scope  = r.scope,
      path   = r.path,
      exists = exists(r.path),
    })
  end

  -- ── Installed Plugins (from ~/.opencode/package.json) ───────
  local decoded = read_json(home .. "/.opencode/package.json")
  if decoded and type(decoded.dependencies) == "table" then
    for name, version in pairs(decoded.dependencies) do
      table.insert(data.plugins, {
        name    = name,
        scope   = "Global",
        version = version,
        path    = "",
      })
    end
  end

  return data
end

--- Dispatch scanner by agent index.
--- @param agent_idx integer  1=Claude, 2=Cursor, 3=Copilot, 4=OpenCode
--- @param cwd string
--- @return table
function M.scan(agent_idx, cwd)
  if agent_idx == 2 then return M.scan_cursor(cwd) end
  if agent_idx == 3 then return M.scan_copilot(cwd) end
  if agent_idx == 4 then return M.scan_opencode(cwd) end
  return M.scan_claude(cwd)
end

return M
