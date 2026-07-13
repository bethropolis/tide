-- file_picker.lua
-- A Lua plugin example that uses the Picker overlay to open files.
-- Register a :fpick command that shows a file picker starting from ".".

tide.register_command("fpick", function(args)
    local dir = args[1] or "."
    -- read directory contents via a simple shell command
    local handle = io.popen("ls -1 " .. dir .. " 2>/dev/null")
    if not handle then
        tide.set_status_message("Failed to read directory: " .. dir)
        return
    end

    local items = {}
    for entry in handle:lines() do
        -- skip hidden files
        if entry:sub(1, 1) ~= "." then
            table.insert(items, {label = entry, value = dir .. "/" .. entry})
        end
    end
    handle:close()

    if #items == 0 then
        tide.set_status_message("No files in " .. dir)
        return
    end

    tide.show_picker("Files (" .. dir .. ")", items, function(selected)
        tide.open_file(selected)
    end)
end)
