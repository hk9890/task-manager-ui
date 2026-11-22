# 0004: A launcher starts one detached process and returns at once

A launched tool outlives the app. A launcher does not wait for a result, poll or coordinate: work
on a tool after its start belongs outside the app.

Rejected: a launcher that waits for its process to end, parses its output, polls it or coordinates
several, because each addition is reasonable alone and together they recreate orchestration.

Rejected: the built-in `agent` launcher, a shell script that started a coding agent, because a
launcher reports only that the process started, so a start that failed inside the script was
silent. An agent that the app shows starts through the agent host (`internal/agent`).
