# agent-tally.nvim

A Neovim plugin and system-wide daemon that tracks AI token usage to your files and tool activity across your projects. It monitors file I/O and command execution from AI coding assistants like Claude Code, Cursor, and Copilot, giving you a clear picture of "tokens in" (context read), "tokens out" (code generated), and the specific tools used to get the job done.

**Dashboard** — Your high-level summary of tokens, agents, and top files at a glance.
![Dashboard](images/Dashboard.png)

**Heatmap** — Daily token activity heatmap. You can generate it with your choice of scope, agent, and metric.
![Heatmap](images/Heatmap.png)

**Agent detail** — Deep dive into an agent’s token history and file interactions.
![AgentDetail](images/AgentDetail.png)

**File detail** — Track exactly who edited a file, when, and the associated token cost.
![FileDetail](images/FileDetail.png)

**Tool usage** — Aggregated count of every tool call (Read, Edit, Bash, Grep, …) made by each agent.
![ToolUsage](images/ToolUsage.png)

**All events** — A complete, chronological log of every system event and timestamp.
![AllEvents](images/AllEvents.png)

## Requirements

- Neovim >= 0.10
- Go >= 1.21 

## Installation

### 1. Install the Neovim plugin

**lazy.nvim**

```lua
{
  "BinL233/agent-tally.nvim",
  config = function()
    require("agent-tally").setup({
      -- auto_start = true,  -- optionally start the daemon with Neovim
    })
  end,
}
```

**packer.nvim**

```lua
use {
  "BinL233/agent-tally.nvim",
  config = function()
    require("agent-tally").setup()
  end,
}
```

**vim-plug**

```vim
Plug 'BinL233/agent-tally.nvim'

" In your init.vim / init.lua:
lua require("agent-tally").setup()
```

### 2. Build the sidecar daemon

```sh
cd ~/.local/share/nvim/lazy/agent-tally.nvim  # or wherever your plugin manager clones to
make build  # Go required
```

This produces `sidecar/build/agent-tallyd`.

### 3. Install the daemon binary

```sh
sudo make install
```

This copies `agent-tallyd` to `/usr/local/bin/` (requires root).
Alternative: `sudo cp sidecar/build/agent-tallyd /usr/local/bin/`

If you don't have root access, install to your user bin instead:

```sh
mkdir -p ~/.local/bin
cp sidecar/build/agent-tallyd ~/.local/bin/agent-tallyd
```

Then make sure `~/.local/bin` is in your `$PATH` 
- You can add `export PATH="$HOME/.local/bin:$PATH"` to your shell rc file.

## Compatible AI Agents

The following agents are monitored by default. Use `:AgentTallyWatchlist` to enable/disable individual tools.

| Agent | Process Name |
|-------|-------------|
| [Claude Code](https://claude.ai/code) | `claude` |
| [Cursor](https://cursor.sh) | `cursor` | 
| [GitHub Copilot](https://github.com/features/copilot) | `copilot` |

Any other CLI tool can be added via `:AgentTallyWatchlist`, just enter the process name as it appears in `ps`.

## How It Works

Agent Tally monitors file changes in your watched directories. When an AI tool writes to a file:
- **Tokens In**: Estimated tokens the AI read from the existing file content
- **Tokens Out**: Estimated tokens the AI generated and wrote to the file

> **Note**: Token counts are estimates based on file size changes (~4 bytes per token). They represent file-level I/O only and do not include conversation context, system prompts, or extended thinking tokens that AI models may use internally.

## Usage

### Commands

| Command                | Description                              |
|------------------------|------------------------------------------|
| `:AgentTally`          | Open the dashboard (auto-starts daemon)  |
| `:AgentTallyStart`     | Start the sidecar daemon manually        |
| `:AgentTallyStop`      | Stop the sidecar daemon                  |
| `:AgentTallyStatus`    | Show daemon status and watchlist         |
| `:AgentTallyWatchlist` | Toggle which AI tools to monitor         |
| `:AgentTallyClean`     | Clean recorded events in the current directory | 
| `:AgentTallyCleanAll`     | Clean all recorded events |

### Running the daemon standalone

```sh
agent-tallyd                                    # watch cwd, default settings
agent-tallyd --watch ~/projects                 # watch a specific directory
agent-tallyd --watch ~/proj1,~/proj2            # watch multiple directories
agent-tallyd --depth 5                          # limit recursive depth
agent-tallyd --db ~/my-events.db                # custom database location
agent-tallyd --socket /tmp/my.sock              # custom socket path
```

### Dashboard keybindings

| Key         | Action                                          |
|-------------|-------------------------------------------------|
| `q` / `Esc` | Close dashboard                                 |
| `r`         | Refresh data                                    |
| `G`         | Grep / filter entries                           |
| `Enter`     | Drill into detail (event, file, or full table)  |
| `Backspace` | Go back to previous view                        |
| `Ctrl-j`    | Next entry                                      |
| `Ctrl-k`    | Previous entry                                  |
| `H`         | Generate heatmap (scope → agent → metric)       |

### Configuration

```lua
require("agent-tally").setup({
  -- Path to the agent-tallyd binary (default: "agent-tallyd")
  daemon_bin = "agent-tallyd",

  -- UNIX socket path, must match the daemon's --socket flag
  socket_path = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.sock",

  -- PID file path used to prevent duplicate daemon instances
  pid_file = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.pid",

  -- Auto-start the daemon when Neovim opens (default: false)
  auto_start = false,

  -- Status line format (%t = total tokens, %p = process name)
  statusline_format = " [AT: %t tokens]",

  -- Query limits 
  query = {
    events_limit = 500,  -- max events loaded into the dashboard per open
    skills_limit = 50,   -- max skill rows fetched for the By Skill section
  },

  -- UI options
  ui = {
    width = 0.8,        -- 80% of editor width
    height = 0.8,       -- 80% of editor height
    border = "rounded", -- border style
  },

  -- Dashboard keymaps
  keymaps = {
    close = { "q", "<Esc>" },
    drill_down = "<CR>",
    back = "<BS>",
    next_entry = "<C-j>",
    prev_entry = "<C-k>",
    grep = "G",
    refresh = "r",
    heatmap = "H",  -- generate heatmap (scope → agent → metric)
  },
})
```

## TODO

- [ ] avante.nvim integration

## License
[MIT](LICENSE)
