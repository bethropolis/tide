; Go Highlights Query

; --- Keywords ---
"package" @keyword
"import" @keyword
"func" @keyword
"type" @keyword
"struct" @keyword
"interface" @keyword
"const" @keyword
"var" @keyword
"for" @keyword
"range" @keyword.control.repeat
"if" @keyword.control.conditional
"else" @keyword.control.conditional
"switch" @keyword.control.conditional
"case" @keyword.control.conditional
"default" @keyword.control.conditional ; Add default case
"fallthrough" @keyword.control
"return" @keyword.control.flow
"go" @keyword.control.concurrent
"defer" @keyword.control.defer
"select" @keyword.control.concurrent
"break" @keyword.control.flow
"continue" @keyword.control.flow
"goto" @keyword.control.flow
"map" @keyword.type       ; 'map' keyword for map types
"chan" @keyword.type      ; 'chan' keyword for channel types


; --- Literals & Constants ---
(raw_string_literal) @string
(interpreted_string_literal) @string
(rune_literal) @string.special ; Character literal
(int_literal) @number
(float_literal) @number
(imaginary_literal) @number
(nil) @constant.builtin
(true) @constant.builtin
(false) @constant.builtin
(iota) @constant.builtin


; --- Comments ---
(comment) @comment


; --- Types ---
(type_identifier) @type           ; User-defined type names


; Match identifiers that are built-in types
(identifier) @type.builtin
(#match? @type.builtin "^(bool|byte|complex64|complex128|error|float32|float64|int|int8|int16|int32|int64|rune|string|uint|uint8|uint16|uint32|uint64|uintptr)$")

; --- Functions & Methods ---
; Function definition names
(function_declaration name: (identifier) @function.definition)
; Method definition names
(method_declaration name: (field_identifier) @function.definition)

; General function calls (will be overridden by built-in rule below if applicable)
(call_expression function: (identifier) @function.call)
; Method calls (e.g., obj.Method())
(call_expression function: (selector_expression field: (field_identifier)) @function.call)

; Built-in function calls (This rule is more specific due to the predicate, overriding the general @function.call)
(call_expression
  function: (identifier) @function.builtin
  (#match? @function.builtin "^(make|len|cap|new|append|copy|delete|panic|recover|complex|real|imag)$")
)

; --- Variables & Identifiers ---
; General identifiers (could refine later if needed)
(identifier) @variable ; Default style for identifiers (might be overridden by func/type)
(blank_identifier) @variable ; The blank identifier '_'

; Struct field definitions
(field_declaration name: (field_identifier) @variable.member) ; field name in struct def


; --- Operators ---
; Assign specific style to operators
"=" @operator
":=" @operator
"+" @operator
"-" @operator
"*" @operator
"/" @operator
"%" @operator
"&" @operator
"|" @operator
"^" @operator
"<<" @operator
">>" @operator
"&^" @operator
"+=" @operator
"-=" @operator
"*=" @operator
"/=" @operator
"%=" @operator
"&=" @operator
"|=" @operator
"^=" @operator
"<<=" @operator
">>=" @operator
"&^=" @operator
"&&" @operator
"||" @operator
"<-" @operator ; Channel operator
"!" @operator
"==" @operator
"!=" @operator
"<" @operator
"<=" @operator
">" @operator
">=" @operator
(pointer_type "*" @operator) ; Pointer star in type definitions


; --- Punctuation ---
; You can style punctuation if desired, e.g.:
; "." @punctuation.delimiter
; "," @punctuation.delimiter
; ";" @punctuation.delimiter
; ":" @punctuation.delimiter
; "(" @punctuation.bracket
; ")" @punctuation.bracket
; "{" @punctuation.bracket
; "}" @punctuation.bracket
; "[" @punctuation.bracket
; "]" @punctuation.bracket

; --- Package Names --- 
(package_clause (package_identifier) @namespace) ; 'package main' -> main
(import_spec (interpreted_string_literal) @string.import) ; capture the path string
(import_spec (package_identifier) @namespace) ; capture the alias/name


; --- Escape Sequences within Strings ---
(escape_sequence) @string.escape


; --- Placeholders (if your grammar supports them) ---
; (placeholder) @text.todo


; --- Labels (rarely used but part of Go) ---
(labeled_statement (label_name) @label)
(goto_statement (label_name) @label)
(break_statement (label_name) @label)
(continue_statement (label_name) @label)
