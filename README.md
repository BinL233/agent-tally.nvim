# agent-tally.nvim

A Neovim plugin and system-wide daemon that tracks AI token utilization across Neovim and CLI tools (Copilot, Claude Code, Aider, etc.).

## Requirements

- Neovim >= 0.10
- Go >= 1.21 (only needed to build the daemon; SQLite is embedded, no system dependency)

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
make build
```

This produces `sidecar/build/agent-tallyd`.

### 3. Install the daemon binary

```sh
make install
```

This copies `agent-tallyd` to `~/.local/bin/`. Make sure `~/.local/bin` is in your `$PATH`.

Alternatively, copy the binary anywhere on your `$PATH`:

```sh
cp sidecar/build/agent-tallyd /usr/local/bin/
```

## Usage

### Commands

| Command                | Description                              |
|------------------------|------------------------------------------|
| `:AgentTally`          | Open the dashboard (auto-starts daemon)  |
| `:AgentTallyStart`     | Start the sidecar daemon manually        |
| `:AgentTallyStop`      | Stop the sidecar daemon                  |
| `:AgentTallyStatus`    | Show daemon status and watchlist         |
| `:AgentTallyWatchlist` | Toggle which AI tools to monitor         |
| `:AgentTallyClear`     | Clear all recorded events (with confirm) |

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

  -- UI options
  ui = {
    width = 0.8,        -- 80% of editor width
    height = 0.8,       -- 80% of editor height
    border = "rounded", -- border style
  },

  -- Dashboard keymaps (all customizable)
  keymaps = {
    close = { "q", "<Esc>" },
    drill_down = "<CR>",
    back = "<BS>",
    next_entry = "<C-j>",
    prev_entry = "<C-k>",
    grep = "G",
    refresh = "r",
  },
})
```

## License
[MIT](LICENSE)
