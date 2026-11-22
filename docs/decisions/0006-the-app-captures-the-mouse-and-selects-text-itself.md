# 0006: The app captures the mouse and selects text itself

Mouse capture stops the drag-select of the terminal. So the app selects a box of the screen and
sends it to the clipboard; shift+drag stays the selection of the terminal.

Rejected: text selection through shift+drag of the terminal only, because a plain drag must still
select text.

Rejected: the `bubblezone` library for hit testing, because its string markers are a risk for the
code of the app that splices ANSI text itself.
