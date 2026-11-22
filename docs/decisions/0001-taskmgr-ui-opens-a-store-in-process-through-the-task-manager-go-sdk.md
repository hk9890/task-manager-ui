# 0001: taskmgr-ui opens a store in-process, through the task-manager Go SDK

The app needs no server, no daemon and no tracker binary at runtime. The SDK serializes the writes
to a store, so the app holds no lock of its own.

Rejected: a tracker command line as a child process for each read and write, because each call
starts a process and opens the store cold.
