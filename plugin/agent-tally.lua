if vim.g.loaded_agent_tally then
  return
end
vim.g.loaded_agent_tally = true

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
