; extends

(inline_link
  "[" @lb
  (#set! @lb conceal ""))

(inline_link
  "]" @rb
  (#set! @rb conceal ""))

(inline_link
  "(" @lp
  (#set! @lp conceal ""))

(inline_link
  (link_destination) @dest
  (#set! @dest conceal ""))

(inline_link
  ")" @rp
  (#set! @rp conceal ""))
