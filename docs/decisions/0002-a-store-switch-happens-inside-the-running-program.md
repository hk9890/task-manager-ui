# 0002: A store switch happens inside the running program

The store picker is a full screen of the one Bubble Tea program, and a switch builds the
store-bound parts again without leaving the program. The cost: store resolution moves inside the
app, and `cmd/` keeps the first resolution only.

Rejected: a separate picker program in `cmd/` that the app quits back to for each switch, because
two Bubble Tea programs must then be sized and styled in parallel.
