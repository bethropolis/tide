; JSON Highlights Query

(string) @string
(escape_sequence) @string.escape

(number) @number.float

(true) @constant.builtin
(false) @constant.builtin  
(null) @constant.builtin

"{" @punctuation.bracket
"}" @punctuation.bracket
"[" @punctuation.bracket
"]" @punctuation.bracket
":" @punctuation.delimiter
"," @punctuation.delimiter

; Highlight object keys more specifically
(pair
    key: (string) @property)

; Error handling
(ERROR) @error
