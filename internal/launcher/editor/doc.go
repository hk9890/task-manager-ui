// Package editor contains rich issue document editor launch flows.
//
// # PrepareDocument → tea.Exec → ApplyEdits contract
//
// The editor flow is split into three phases so that Bubble Tea's tea.Exec can
// hand terminal control to the external editor without TTY contention:
//
//  1. PrepareDocument(ctx, issueID) — calls ShowIssue, renders the edit
//     document, and writes it to a temp file. Returns a Prepared value
//     carrying the temp path and the original issue.
//
//  2. tea.Exec (model layer) — the model calls BuildEditorCmd(path) to get an
//     *exec.Cmd, wraps it via ExecCommandFactory, and returns it from Update
//     as tea.Exec(editorCmd, callback). Bubble Tea suspends the TUI, hands the
//     terminal to the editor process, and restores the TUI when the editor exits.
//     The callback delivers an editorExitedMsg to the Update loop.
//
//  3. ApplyEdits(ctx, issueID, issue, path) — reads the temp file, parses the
//     document, diffs it against the original, and calls UpdateIssue when
//     changed. The temp file is removed on all paths (success, no-change, and
//     parse/gateway error).
//
// Editor exit code != 0, parse failure, and issue-deleted-between-prepare-and-apply
// each surface as an error in editIssueResultMsg, shown by the existing error toast.
package editor
