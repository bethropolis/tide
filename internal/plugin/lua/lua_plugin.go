package lua

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"

	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/tui"
	"github.com/bethropolis/tide/internal/types"
	lua "github.com/yuin/gopher-lua"
)

type LuaPlugin struct {
	name          string
	script        string
	api           plugin.EditorAPI
	L             *lua.LState
	mu            sync.Mutex
	subscriptions map[event.Type][]event.SubscriptionID
}

// NewLuaPlugin creates a new Lua plugin from a script path.
func NewLuaPlugin(path string) (*LuaPlugin, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lua script: %w", err)
	}

	// Use filename without extension as plugin name for now
	name := filepath.Base(path)
	name = name[:len(name)-len(filepath.Ext(name))]

	return &LuaPlugin{
		name:   name,
		script: string(content),
	}, nil
}

func (p *LuaPlugin) Name() string {
	return "lua_" + p.name
}

func (p *LuaPlugin) Initialize(api plugin.EditorAPI) error {
	p.api = api
	p.L = lua.NewState()
	p.subscriptions = make(map[event.Type][]event.SubscriptionID)

	p.registerAPI()

	p.mu.Lock()
	err := p.L.DoString(p.script)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("lua execution error: %w", err)
	}

	return nil
}

func (p *LuaPlugin) Shutdown() error {
	for eventType, ids := range p.subscriptions {
		for _, id := range ids {
			p.api.UnsubscribeEvent(eventType, id)
		}
	}
	p.subscriptions = nil

	if p.L != nil {
		p.L.Close()
	}
	return nil
}

// convertEventToLua translates Go event data into a Lua table so that
// plugins can inspect cursor positions, edit boundaries, file paths, etc.
// Returns lua.LNil when the event carries no data payload.
func (p *LuaPlugin) convertEventToLua(e event.Event) lua.LValue {
	data := e.Data
	if data == nil {
		return lua.LNil
	}

	tbl := p.L.NewTable()

	switch d := data.(type) {
	case event.CursorMovedData:
		p.L.SetField(tbl, "old_line", lua.LNumber(d.OldPosition.Line))
		p.L.SetField(tbl, "old_col", lua.LNumber(d.OldPosition.Col))
		p.L.SetField(tbl, "new_line", lua.LNumber(d.NewPosition.Line))
		p.L.SetField(tbl, "new_col", lua.LNumber(d.NewPosition.Col))
		return tbl

	case event.BufferModifiedData:
		editTbl := p.L.NewTable()
		p.L.SetField(editTbl, "start_line", lua.LNumber(d.Edit.StartPosition.Row))
		p.L.SetField(editTbl, "start_col", lua.LNumber(d.Edit.StartPosition.Column))
		p.L.SetField(editTbl, "old_end_line", lua.LNumber(d.Edit.OldEndPosition.Row))
		p.L.SetField(editTbl, "old_end_col", lua.LNumber(d.Edit.OldEndPosition.Column))
		p.L.SetField(editTbl, "new_end_line", lua.LNumber(d.Edit.NewEndPosition.Row))
		p.L.SetField(editTbl, "new_end_col", lua.LNumber(d.Edit.NewEndPosition.Column))
		p.L.SetField(tbl, "edit", editTbl)
		return tbl

	case event.BufferSavedData:
		p.L.SetField(tbl, "file_path", lua.LString(d.FilePath))
		return tbl

	case event.BufferLoadedData:
		p.L.SetField(tbl, "file_path", lua.LString(d.FilePath))
		return tbl

	case event.KeyPressedData:
		if d.KeyEvent != nil {
			p.L.SetField(tbl, "key", lua.LString(d.KeyEvent.Name()))
		}
		return tbl

	case event.ThemeChangedData:
		p.L.SetField(tbl, "old_theme", lua.LString(d.OldThemeName))
		p.L.SetField(tbl, "new_theme", lua.LString(d.NewThemeName))
		return tbl
	}

	return lua.LNil
}

func (p *LuaPlugin) registerAPI() {
	// Create the `tide` table
	tideTable := p.L.NewTable()

	// tide.set_status_message(msg)
	p.L.SetField(tideTable, "set_status_message", p.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		p.api.SetStatusMessage(msg)
		return 0
	}))

	// tide.register_command(name, callback)
	p.L.SetField(tideTable, "register_command", p.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		callback := L.CheckFunction(2)

		err := p.api.RegisterCommand(name, func(args []string) error {
			p.mu.Lock()
			defer p.mu.Unlock()

			// Call the Lua function
			p.L.Push(callback)

			// Pass arguments as a Lua table
			argTable := p.L.NewTable()
			for i, arg := range args {
				p.L.RawSetInt(argTable, i+1, lua.LString(arg))
			}
			p.L.Push(argTable)

			if err := p.L.PCall(1, 0, nil); err != nil {
				logger.Errorf("Lua command '%s' error: %v", name, err)
				p.api.SetStatusMessage("Lua error: %v", err)
				return fmt.Errorf("lua command error: %w", err)
			}
			return nil
		})

		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}

		L.Push(lua.LNil)
		return 1
	}))

	// tide.get_cursor() -> line, col
	p.L.SetField(tideTable, "get_cursor", p.L.NewFunction(func(L *lua.LState) int {
		pos := p.api.GetCursor()
		L.Push(lua.LNumber(pos.Line))
		L.Push(lua.LNumber(pos.Col))
		return 2
	}))

	// tide.set_cursor(line, col)
	p.L.SetField(tideTable, "set_cursor", p.L.NewFunction(func(L *lua.LState) int {
		line := L.CheckInt(1)
		col := L.CheckInt(2)
		p.api.SetCursor(types.Position{Line: line, Col: col})
		return 0
	}))

	// tide.get_buffer_lines(start_line, end_line)
	p.L.SetField(tideTable, "get_buffer_lines", p.L.NewFunction(func(L *lua.LState) int {
		startLine := L.CheckInt(1)
		endLine := L.CheckInt(2)
		lines, err := p.api.GetBufferLines(startLine, endLine)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		luaTable := L.NewTable()
		for i, line := range lines {
			L.RawSetInt(luaTable, i+1, lua.LString(string(line)))
		}
		L.Push(luaTable)
		return 1
	}))

	// tide.insert_text(line, col, text)
	p.L.SetField(tideTable, "insert_text", p.L.NewFunction(func(L *lua.LState) int {
		line := L.CheckInt(1)
		col := L.CheckInt(2)
		text := L.CheckString(3)
		err := p.api.InsertText(types.Position{Line: line, Col: col}, []byte(text))
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.delete_range(start_line, start_col, end_line, end_col)
	p.L.SetField(tideTable, "delete_range", p.L.NewFunction(func(L *lua.LState) int {
		sl := L.CheckInt(1)
		sc := L.CheckInt(2)
		el := L.CheckInt(3)
		ec := L.CheckInt(4)
		err := p.api.DeleteRange(types.Position{Line: sl, Col: sc}, types.Position{Line: el, Col: ec})
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.get_buffer_file_path()
	p.L.SetField(tideTable, "get_buffer_file_path", p.L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(p.api.GetBufferFilePath()))
		return 1
	}))

	// tide.open_file(filepath)
	p.L.SetField(tideTable, "open_file", p.L.NewFunction(func(L *lua.LState) int {
		filepath := L.CheckString(1)
		p.api.OpenFile(filepath)
		return 0
	}))

	// tide.next_buffer()
	p.L.SetField(tideTable, "next_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.NextBuffer()
		return 0
	}))

	// tide.prev_buffer()
	p.L.SetField(tideTable, "prev_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.PrevBuffer()
		return 0
	}))

	// tide.close_buffer()
	p.L.SetField(tideTable, "close_buffer", p.L.NewFunction(func(L *lua.LState) int {
		err := p.api.CloseBuffer()
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.force_close_buffer()
	p.L.SetField(tideTable, "force_close_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.ForceCloseBuffer()
		return 0
	}))

	// tide.show_picker(title, items, on_select [, on_cancel])
	// Each item is a Lua table with optional fields: label, description, value.
	p.L.SetField(tideTable, "show_picker", p.L.NewFunction(func(L *lua.LState) int {
		title := L.CheckString(1)
		itemsTable := L.CheckTable(2)
		onSelect := L.CheckFunction(3)

		var onCancel func()
		if L.GetTop() >= 4 {
			cf := L.CheckFunction(4)
			onCancel = func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.L.Push(cf)
				if err := p.L.PCall(0, 0, nil); err != nil {
					logger.Errorf("Lua picker on_cancel error: %v", err)
				}
			}
		}

		var items []tui.PickerItem
		itemsTable.ForEach(func(_, v lua.LValue) {
			tbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			items = append(items, tui.PickerItem{
				Label:       tbl.RawGetString("label").String(),
				Description: tbl.RawGetString("description").String(),
				Value:       tbl.RawGetString("value").String(),
			})
		})

		wrappedSelect := func(val string) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.L.Push(onSelect)
			p.L.Push(lua.LString(val))
			if err := p.L.PCall(1, 0, nil); err != nil {
				logger.Errorf("Lua picker on_select error: %v", err)
			}
		}

		p.api.ShowPicker(title, items, wrappedSelect, onCancel)

		return 0
	}))

	// tide.subscribe(event_name, callback) -> id
	p.L.SetField(tideTable, "subscribe", p.L.NewFunction(func(L *lua.LState) int {
		eventName := L.CheckString(1)
		callback := L.CheckFunction(2)

		var eventType event.Type
		switch eventName {
		case "buffer_modified":
			eventType = event.TypeBufferModified
		case "buffer_loaded":
			eventType = event.TypeBufferLoaded
		case "buffer_saved":
			eventType = event.TypeBufferSaved
		case "cursor_moved":
			eventType = event.TypeCursorMoved
		case "theme_changed":
			eventType = event.TypeThemeChanged
		case "key_pressed":
			eventType = event.TypeKeyPressed
		case "app_ready":
			eventType = event.TypeAppReady
		case "app_quit":
			eventType = event.TypeAppQuit
		default:
			L.ArgError(1, fmt.Sprintf("unknown event type: %s", eventName))
			return 0
		}

		id := p.api.SubscribeEvent(eventType, func(e event.Event) bool {
			go func() {
				p.mu.Lock()
				defer p.mu.Unlock()

				p.L.Push(callback)
				p.L.Push(p.convertEventToLua(e))

				if err := p.L.PCall(1, 0, nil); err != nil {
					logger.Errorf("Lua event '%s' callback error: %v", eventName, err)
				}
			}()
			return false
		})

		p.subscriptions[eventType] = append(p.subscriptions[eventType], id)

		L.Push(lua.LNumber(id))
		return 1
	}))

	// tide.unsubscribe(id) -> bool
	p.L.SetField(tideTable, "unsubscribe", p.L.NewFunction(func(L *lua.LState) int {
		id := event.SubscriptionID(L.CheckNumber(1))

		for eventType, ids := range p.subscriptions {
			for i, subID := range ids {
				if subID == id {
					p.api.UnsubscribeEvent(eventType, id)
					p.subscriptions[eventType] = append(ids[:i], ids[i+1:]...)
					L.Push(lua.LTrue)
					return 1
				}
			}
		}

		L.Push(lua.LFalse)
		return 1
	}))

	// Register it globally
	p.L.SetGlobal("tide", tideTable)
}
