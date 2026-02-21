tide.set_status_message("Lua plugin loaded!")

tide.subscribe("app_ready", function()
    tide.set_status_message("App is ready! Hello from Lua")
end)

tide.register_command("hello", function(args)
    tide.set_status_message("Hello command invoked from Lua! Arguments: " .. table.concat(args, ", "))
end)

tide.register_command("lua_stats", function(args)
    local line, col = tide.get_cursor()
    local filepath = tide.get_buffer_file_path()
    tide.set_status_message("File: " .. tostring(filepath) .. " Cursor: L" .. tostring(line) .. " C" .. tostring(col))
end)

tide.register_command("insert_lua", function(args)
    local line, col = tide.get_cursor()
    tide.insert_text(line, col, "This text was inserted by a Lua script!")
end)
