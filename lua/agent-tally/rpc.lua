local M = {}

local uv = vim.uv or vim.loop

--- Send a JSON-RPC request to the daemon over the UNIX socket.
---@param socket_path string
---@param method string
---@param params? table
---@param callback fun(err: string?, result: any)
function M.request(socket_path, method, params, callback)
  local client = uv.new_pipe(false)

  client:connect(socket_path, function(err)
    if err then
      local msg = "connect: " .. err
      if err:match("ENOENT") then
        msg = "Daemon is not running. Start it with :AgentTallyStart"
      end
      callback(msg, nil)
      return
    end

    local payload = vim.json.encode({
      method = method,
      params = params,
    }) .. "\n"

    client:write(payload, function(write_err)
      if write_err then
        client:close()
        callback("write: " .. write_err, nil)
        return
      end
    end)

    local chunks = {}
    client:read_start(function(read_err, data)
      if read_err then
        client:close()
        callback("read: " .. read_err, nil)
        return
      end

      if data then
        table.insert(chunks, data)
      else
        -- EOF: parse accumulated response.
        client:close()
        local raw = table.concat(chunks)
        local ok, resp = pcall(vim.json.decode, raw)
        if not ok then
          callback("invalid json: " .. raw, nil)
          return
        end
        if resp.error then
          callback(resp.error, nil)
        else
          callback(nil, resp.result)
        end
      end
    end)
  end)
end

return M
