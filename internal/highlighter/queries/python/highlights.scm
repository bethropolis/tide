; Python Highlights Query

; Keywords
"def" @keyword.function
"class" @keyword
"return" @keyword.control.return
"yield" @keyword.control.yield
"if" @keyword.control.conditional
"else" @keyword.control.conditional
"elif" @keyword.control.conditional
"for" @keyword.control.repeat
"while" @keyword.control.repeat
"in" @keyword.operator
"try" @keyword.control.exception
"except" @keyword.control.exception
"finally" @keyword.control.exception
"with" @keyword.control.context
"as" @keyword
"import" @keyword.control.import
"from" @keyword.control.import
"pass" @keyword.control.flow
"break" @keyword.control.flow
"continue" @keyword.control.flow
"raise" @keyword.control.exception
"assert" @keyword.control.exception
"del" @keyword
"global" @keyword.storage
"nonlocal" @keyword.storage
"lambda" @keyword.function

; Functions and variables
(identifier) @variable

; Function calls
(call
    function: (identifier) @function.call)

; Method calls
(call
    function: (attribute 
        object: (_) 
        attribute: (identifier) @method.call))

; Function definitions
(function_definition
    name: (identifier) @function.definition)

; Class definitions
(class_definition
    name: (identifier) @type.definition)

; Literals
(string) @string
(comment) @comment
(integer) @number
(float) @number

; Constants
(true) @constant.builtin
(false) @constant.builtin
(none) @constant.builtin

; Types
(type
    (identifier) @type)

; Operators
"=" @operator
"+" @operator
"-" @operator
"*" @operator
"/" @operator
"%" @operator
"**" @operator
"//" @operator
"@" @operator
"~" @operator
"&" @operator
"|" @operator
"^" @operator
"<<" @operator
">>" @operator
"+=" @operator
"-=" @operator
"*=" @operator
"/=" @operator
"%=" @operator
"**=" @operator
"//=" @operator
"@=" @operator
"&=" @operator
"|=" @operator
"^=" @operator
"<<=" @operator
">>=" @operator
"==" @operator
"!=" @operator
"<" @operator
"<=" @operator
">" @operator
">=" @operator
"and" @operator.logical
"or" @operator.logical
"not" @operator.logical
"is" @operator
"is not" @operator

; Punctuation
"[" @punctuation.bracket
"]" @punctuation.bracket
"{" @punctuation.bracket
"}" @punctuation.bracket
"(" @punctuation.bracket
")" @punctuation.bracket
"." @punctuation.delimiter
"," @punctuation.delimiter
":" @punctuation.delimiter
