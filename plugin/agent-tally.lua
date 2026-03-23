if vim.g.loaded_agent_tally then
  return
end
vim.g.loaded_agent_tally = true

-- Highlight groups for the dashboard.
-- Each group falls back to a semantic built-in so it works on any theme.
local function def_hl(name, opts)
  vim.api.nvim_set_hl(0, name, opts)
end

-- Section 1: Status / Database / Watchlist / Paths / Active  →  teal/cyan
def_hl("AgentTallySection1", { fg = "#7dcfff", bold = true, default = true })
-- Section 2: Tokens In / Tokens Out / Total / Events          →  yellow/amber
def_hl("AgentTallySection2", { fg = "#e0af68", bold = true, default = true })
-- Section 3: By Process table header + separator              →  purple
def_hl("AgentTallySection3", { fg = "#bb9af7", bold = true, default = true })
-- Section 4: Recent Events table header + separator           →  green
def_hl("AgentTallySection4", { fg = "#9ece6a", bold = true, default = true })
-- Section 5: By File table header + separator                 →  orange
def_hl("AgentTallySection5", { fg = "#ff9e64", bold = true, default = true })
-- Keybind hint line at the bottom                             →  dim/comment
def_hl("AgentTallyHint",     { link = "Comment", default = true })

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
