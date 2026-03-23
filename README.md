# agent-tally.nvim

A Neovim plugin and system-wide daemon that tracks AI token utilization and skill usage across Neovim and CLI tools (Copilot, Claude Code, Aider, etc.).

## Requirements

- Neovim >= 0.10 
- Go >= 1.21
- SQLite

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

| Command             | Description                          |
|---------------------|--------------------------------------|
| `:AgentTallyStart`  | Start the sidecar daemon             |
| `:AgentTallyStop`   | Stop the sidecar daemon              |
| `:AgentTallyStatus` | Show daemon status and watchlist     |
| `:AgentTally`       | Open the tally dashboard             |

### Running the daemon standalone

```sh
agent-tallyd                          # uses default paths
agent-tallyd --db ~/my-events.db      # custom database location
agent-tallyd --socket /tmp/my.sock    # custom socket path
```

### Configuration

```lua
require("agent-tally").setup({
  -- Path to the agent-tallyd binary (default: "agent-tallyd")
  daemon_bin = "agent-tallyd",

  -- UNIX socket path, must match the daemon's --socket flag
  socket_path = (os.getenv("XDG_RUNTIME_DIR") or "/tmp") .. "/agent-tally.sock",

  -- Auto-start the daemon when Neovim opens (default: false)
  auto_start = false,

  -- Status line format (%t = total tokens, %p = process name)
  statusline_format = " [AT: %t tokens]",
})
```

## License
[MIT](LICENSE)
