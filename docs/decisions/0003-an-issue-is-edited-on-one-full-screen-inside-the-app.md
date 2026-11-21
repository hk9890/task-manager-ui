# 0003: An issue is edited on one full screen inside the app

One screen edits an issue with its description, or a comment, and it fits a terminal of 80x24.

Rejected: the hand-off of the terminal to `$EDITOR` for the description, because an issue then has
two editors: its fields in the app and its description in a second program.

Rejected: the modal dialogs that created and updated an issue, because they did not fit a terminal
of fewer than about 39 rows: at 30 rows the labels, the description and the buttons were not drawn.
