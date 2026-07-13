-- statusline.lua
-- A Lua plugin example that shows cursor position on every cursor move.

local last_msg = ""

tide.subscribe("cursor_moved", function(event)
    if event and event.new_line then
        local msg = string.format("L%d C%d", event.new_line, event.new_col)
        if msg ~= last_msg then
            last_msg = msg
            tide.set_status_message(msg)
        end
    end
end)
