if vim.g.loaded_agent_tally then
  return
end
vim.g.loaded_agent_tally = true

-- Highlight groups for the dashboard.
-- Re-applied on ColorScheme so colors survive theme changes.
local function def_hl(name, opts)
  vim.api.nvim_set_hl(0, name, opts)
end

local function setup_highlights()
  -- Section 1: Status / Database / Watchlist / Paths / Active  →  teal/cyan
  def_hl("AgentTallySection1", { fg = "#7dcfff", bold = true })
  -- Section 2: Tokens In / Tokens Out / Total / Events          →  yellow/amber
  def_hl("AgentTallySection2", { fg = "#e0af68", bold = true })
  -- Section 3: By Process table header + separator              →  purple
  def_hl("AgentTallySection3", { fg = "#bb9af7", bold = true })
  -- Section 4: Recent Events table header + separator           →  green
  def_hl("AgentTallySection4", { fg = "#9ece6a", bold = true })
  -- Section 5: By File table header + separator                 →  orange
  def_hl("AgentTallySection5", { fg = "#ff9e64", bold = true })
  -- Section 6: By Tool table header + separator                →  pink/rose
  def_hl("AgentTallySection6", { fg = "#f7768e", bold = true })
  -- Keybind hint line at the bottom                             →  dim/comment
  def_hl("AgentTallyHint",     { link = "Comment" })
  -- Heatmap intensity levels (GitHub-green palette)
  def_hl("AgentTallyHeat0", { fg = "#2d333b" }) -- no activity
  def_hl("AgentTallyHeat1", { fg = "#0e4429" }) -- very low
  def_hl("AgentTallyHeat2", { fg = "#006d32" }) -- low
  def_hl("AgentTallyHeat3", { fg = "#26a641" }) -- medium
  def_hl("AgentTallyHeat4", { fg = "#39d353" }) -- high
  def_hl("AgentTallyHeat5", { fg = "#56e370" }) -- very high
  def_hl("AgentTallyHeat6", { fg = "#7ff0a0" }) -- max
end

setup_highlights()

vim.api.nvim_create_autocmd("ColorScheme", {
  group = vim.api.nvim_create_augroup("AgentTallyHL", { clear = true }),
  pattern = "*",
  callback = setup_highlights,
})

vim.api.nvim_create_user_command("AgentTally", function(opts)
  require("agent-tally").command(opts)
end, { nargs = "*", desc = "Agent Tally dashboard" })

vim.api.nvim_create_user_command("AgentTallyStart", function()
  require("agent-tally").start_daemon()
end, { desc = "Start the agent-tally sidecar daemon" })

vim.api.nvim_create_user_command("AgentTallyStop", function()
  require("agent-tally").stop_daemon()
end, { desc = "Stop the agent-tally sidecar daemon" })

vim.api.nvim_create_user_command("AgentTallyStatus", function()
  require("agent-tally").status()
end, { desc = "Show agent-tally daemon status" })

vim.api.nvim_create_user_command("AgentTallyWatchlist", function()
  require("agent-tally.watchlist").open()
end, { desc = "Configure agent-tally watchlist" })

vim.api.nvim_create_user_command("AgentTallyClear", function()
  require("agent-tally").clear()
end, { desc = "Clear all agent-tally events from the database" })

vim.api.nvim_create_user_command("AgentTallyClean", function()
  require("agent-tally").clean()
end, { desc = "Clear agent-tally events for the current directory" })

vim.api.nvim_create_user_command("AgentTallyCleanAll", function()
  require("agent-tally").clean_all()
end, { desc = "Clear all agent-tally events from the database" })

vim.api.nvim_create_user_command("AgentTallyMockData", function()
  local config = require("agent-tally.config")
  local mock   = require("agent-tally.mock")
  vim.notify("[agent-tally] Inserting mock data...", vim.log.levels.INFO)
  mock.generate(config.current.socket_path, vim.fn.getcwd(), function(ev_ok, sk_ok, first_err, total_errs)
    local msg = string.format("[agent-tally] Done: %d token events, %d tool events inserted", ev_ok, sk_ok)
    if total_errs and total_errs > 0 then
      msg = msg .. string.format(" (%d errors", total_errs)
      if first_err then msg = msg .. ": " .. first_err end
      msg = msg .. ")"
    end
    vim.notify(msg, vim.log.levels.INFO)
  end)
end, { desc = "Insert 3 months of mock token events for testing" })
