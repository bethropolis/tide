; Keywords
(package_clause (package_identifier) @keyword)
(import_declaration (import_keyword) @keyword)
(func_literal (func_keyword) @keyword)
(function_declaration (func_keyword) @keyword)
(type_declaration (type_keyword) @keyword)
(struct_type (struct_keyword) @keyword)
(interface_type (interface_keyword) @keyword)
(const_declaration (const_keyword) @keyword)
(var_declaration (var_keyword) @keyword)
(for_statement (for_keyword) @keyword)
(if_statement (if_keyword) @keyword)
(else_keyword) @keyword
(switch_statement (switch_keyword) @keyword)
(case_clause (case_keyword) @keyword)
(fallthrough_statement (fallthrough_keyword) @keyword)
(return_statement (return_keyword) @keyword)
(go_statement (go_keyword) @keyword)
(defer_statement (defer_keyword) @keyword)
(select_statement (select_keyword) @keyword)
(break_statement (break_keyword) @keyword)
(continue_statement (continue_keyword) @keyword)
(goto_statement (goto_keyword) @keyword)
(map_type (map_keyword) @keyword)
(chan_type (chan_keyword) @keyword)

; Control Flow specific keywords
(for_statement (range_keyword) @keyword.control.repeat) ; like 'range'
(if_statement (else_keyword) @keyword.control.conditional)

; Literals
(raw_string_literal) @string
(interpreted_string_literal) @string
(rune_literal) @string.special ; Often highlighted like strings
(int_literal) @number
(float_literal) @number
(imaginary_literal) @number
(nil) @constant.builtin
(true) @constant.builtin
(false) @constant.builtin
(iota) @constant.builtin

; Comments
(comment) @comment

; Types
(type_identifier) @type
(primitive_type) @type.builtin

; Functions
(call_expression (identifier) @function)
(call_expression (selector_expression field: (field_identifier) @function)) ; method calls like fmt.Println
(function_declaration name: (identifier) @function.definition)
