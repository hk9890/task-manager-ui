# AGENTS.md — taskmgr-ui routing

## Repository purpose

A terminal UI for browsing task-manager issues, creating and updating work, and launching external
tools from issue context. Go and Bubble Tea over the task-manager Go SDK
(`github.com/hk9890/task-manager/sdk/tasks`) — the store is opened in-process, so there is no server,
no database and no tracker subprocess.

## Use-case routing

Every route below is **mandatory, not advisory**. Load the document BEFORE the first action of that
kind — loading it afterwards does not count, and no route becomes skippable because the task looks
small.

### Research, planning, architecture — and finding anything at all

**MUST read [docs/OVERVIEW.md](docs/OVERVIEW.md) before your first `rg`, `grep`, `Glob`, or `ls` of
this repository, and before writing any plan or design.** It is the map — the layout and the search
expressions that locate things fast. Go there instead of grepping blind.

### Coding and file changes

**MUST read [docs/CODING.md](docs/CODING.md) before creating or editing ANY file under `cmd/`,
`internal/`, or `scripts/`, or `.mise.toml` or `.github/workflows/ci.yml`.** It owns the startup
contract, the architectural rules, the naming conventions, and which doc outranks it inside each
area.

### Designing or changing what the operator sees

**MUST read [docs/DESIGN-GUIDE.md](docs/DESIGN-GUIDE.md) before creating or editing ANY file under
`internal/ui/` or `internal/mode/`, and before writing any plan or answer that specifies a screen, a
key, a glyph, or a colour.** It is the interaction law; two of its rules are gated and the rest is
held at review.

### Runtime config, keybindings, launcher templates

**MUST read [docs/CONFIGURATION.md](docs/CONFIGURATION.md) before editing `internal/config/`, and
before adding or changing a config key, a keybinding, or a launcher placeholder.** It owns the config
model, the resolution order, and the interpolation surface.

### Testing and verification

**MUST read [docs/TESTING.md](docs/TESTING.md) before writing a test, and before your first
`mise run test*`, `mise run quality*`, `mise run ci`, or `go test` in this repository.** It owns the
tiers, which one a new test belongs in, the golden-file conventions, and what may be faked.

### Running the app to reproduce a bug or verify a change

**MUST read [docs/RUNNING.md](docs/RUNNING.md) before launching the binary by hand, before driving it
under the PTY capture harness, and before reproducing a reported bug.** It owns the launch commands,
the capture steps, and the gotchas that hang a scripted run.

### Diagnosing a failure, or reading what a past run did

**MUST read [docs/MONITORING.md](docs/MONITORING.md) before reading any taskmgr-ui log, structured
record, or stderr diagnostic capture, and before your first edit made in response to a failed run.**
It owns where the evidence lands, the record shape, the sink's level, and what `--debug` does and
does not mirror.

### Reviewing a PR or a diff

**MUST read [docs/REVIEWING.md](docs/REVIEWING.md) before your first `git diff` or `gh pr diff` run
to judge a change, and whenever a review is requested.** It carries what this repository must check
on top of the generic pass, the severity ladder, and what is explicitly not a finding.

### Writing documentation

**MUST read [docs/DOCUMENTING.md](docs/DOCUMENTING.md) and invoke the
`instruction-writing:writing-project-docs` skill before creating or editing ANY tracked Markdown
file — under `docs/`, at the repository root, or in `.github/`.** It owns the citation gate, the
docs outside the canonical set, and what this repository decided not to document.

### Recording a task, a TODO, or an issue

**MUST read [docs/CHANGE-WORKFLOW.md](docs/CHANGE-WORKFLOW.md) and invoke the `tasks:tasks-writing`
skill before recording any task, TODO or issue** — this project tracks its work in `taskmgr`.

### Commit, branch, worktree, PR, merge

**MUST read [docs/CHANGE-WORKFLOW.md](docs/CHANGE-WORKFLOW.md) before any git command that writes** —
worktree, commit, branch, push — **and before opening a PR.** This route does not fire on read-only
git (`status`, `log`) — a `git diff` to judge a change is gated by REVIEWING.md above. It owns the
worktree-per-change rule, the pre-handoff gates, and the PR requirements.

### Release

**MUST read [docs/RELEASING.md](docs/RELEASING.md) before cutting a release**, before editing
`.github/workflows/release.yml` or `.goreleaser.yaml`, or before adding, renaming or removing a
released artifact.
