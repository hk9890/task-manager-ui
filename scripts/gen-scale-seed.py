#!/usr/bin/env python3
"""
gen-scale-seed.py — one-shot generator for scale-seed.json

Produces internal/testing/e2e/embeddedfixture/scale-seed.json.
This script is a BUILD TOOL, not production code.
Run once; commit the output artifact.

Usage: python3 scripts/gen-scale-seed.py [output-path]
"""

import json
import sys
import os

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DEFAULT_OUTPUT = os.path.join(
    REPO_ROOT, "internal", "testing", "e2e", "embeddedfixture", "scale-seed.json"
)

output_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_OUTPUT

# ---- configuration constants --------------------------------------------------
PREFIX = "bws"

# Shared keyword corpus — each must appear in 20+ titles/descriptions
KEYWORDS = ["workflow", "pipeline", "dashboard"]

# Shared label (20+ issues)
SHARED_LABEL = "scale-fixture"

# Rare labels
RARE_LABELS = ["legacy", "security", "experimental", "debt", "perf"]

# Assignees: a few with many, a few with one
HEAVY_ASSIGNEES = ["alice", "bob", "carol"]
LIGHT_ASSIGNEES = ["dave", "eve", "frank", "grace", "henry"]

# Max-length title (255 chars is typical bd limit)
MAX_TITLE = "A" * 120 + " max-length-title-edge-case"

issues = []
deps = []

next_id = [1]


def new_id():
    i = next_id[0]
    next_id[0] += 1
    return f"{PREFIX}-{i}"


def add_issue(
    title,
    description="",
    issue_type="task",
    priority=2,
    status="open",
    assignee="alice",
    labels=None,
    comments=None,
):
    if labels is None:
        labels = [SHARED_LABEL]
    if comments is None:
        comments = []
    iid = new_id()
    issues.append(
        {
            "id": iid,
            "title": title,
            "description": description,
            "type": issue_type,
            "priority": priority,
            "status": status,
            "assignee": assignee,
            "labels": labels,
            "comments": comments,
        }
    )
    return iid


def add_dep(blocker_id, blocked_id):
    deps.append({"blocker_id": blocker_id, "blocked_id": blocked_id})


# ---- 1. Edge-case issues ------------------------------------------------------

# Emoji title
emoji_id = add_issue(
    title="🚀 Deploy pipeline to production",
    description="This issue has an emoji title for edge-case testing.",
    issue_type="task",
    priority=1,
    status="open",
    assignee="alice",
    labels=[SHARED_LABEL, "edge-case"],
)

# Shell metacharacter title
metachar_id = add_issue(
    title="Fix issue with `rm -rf`; don't 'break' \"things\"",
    description="Shell metacharacter edge case: backtick, semicolon, single quote, double quote.",
    issue_type="bug",
    priority=0,
    status="open",
    assignee="bob",
    labels=[SHARED_LABEL, "edge-case"],
)

# Max-length title
maxlen_id = add_issue(
    title=MAX_TITLE,
    description="Max-length title edge case for text overflow testing.",
    issue_type="task",
    priority=2,
    status="open",
    assignee="carol",
    labels=[SHARED_LABEL, "edge-case"],
)

# Multi-line description
multiline_id = add_issue(
    title="Issue with multi-line description",
    description="Line one of the description.\n\nLine three after blank.\n\nLine five with details:\n- bullet one\n- bullet two\n- bullet three\n\nFinal paragraph for the workflow pipeline dashboard edge case.",
    issue_type="task",
    priority=3,
    status="open",
    assignee="alice",
    labels=[SHARED_LABEL, "edge-case"],
)

# Null/missing description (empty string maps to omitted key in bd)
null_desc_id = add_issue(
    title="Issue with no description (781a regression guard)",
    description="",
    issue_type="bug",
    priority=1,
    status="open",
    assignee="bob",
    labels=[SHARED_LABEL, "edge-case"],
    comments=["Comment on null-description issue"],
)

# ---- 2. Parent chain: 5-deep --------------------------------------------------
# chain: parent_1 -> parent_2 -> parent_3 -> parent_4 -> leaf

chain_root = add_issue(
    title="Epic: top-level workflow initiative",
    description="Root of the 5-deep parent chain for hierarchy testing.",
    issue_type="epic",
    priority=1,
    status="open",
    assignee="alice",
    labels=[SHARED_LABEL],
)

chain_2 = add_issue(
    title="Sub-epic: workflow phase 1",
    description="Level 2 of 5-deep parent chain.",
    issue_type="epic",
    priority=1,
    status="open",
    assignee="alice",
    labels=[SHARED_LABEL],
)

chain_3 = add_issue(
    title="Feature: workflow pipeline integration",
    description="Level 3 of 5-deep parent chain.",
    issue_type="feature",
    priority=2,
    status="open",
    assignee="bob",
    labels=[SHARED_LABEL],
)

chain_4 = add_issue(
    title="Task: implement dashboard pipeline connector",
    description="Level 4 of 5-deep parent chain.",
    issue_type="task",
    priority=2,
    status="in_progress",
    assignee="carol",
    labels=[SHARED_LABEL],
)

chain_leaf = add_issue(
    title="Subtask: write unit tests for pipeline connector",
    description="Level 5 (leaf) of 5-deep parent chain.",
    issue_type="task",
    priority=3,
    status="open",
    assignee="carol",
    labels=[SHARED_LABEL],
)

# Wire parent chain as dependencies (parent blocks child is the convention)
add_dep(chain_root, chain_2)
add_dep(chain_2, chain_3)
add_dep(chain_3, chain_4)
add_dep(chain_4, chain_leaf)

# ---- 3. Parent with 3+ children -----------------------------------------------
parent_3wide = add_issue(
    title="Epic: dashboard redesign workflow",
    description="Parent with 3+ children for width testing.",
    issue_type="epic",
    priority=1,
    status="open",
    assignee="alice",
    labels=[SHARED_LABEL],
)

child_ids = []
for i in range(4):
    cid = add_issue(
        title=f"Child task {i+1}: dashboard component for workflow",
        description=f"Child {i+1} of the 3-wide parent.",
        issue_type="task",
        priority=2,
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )
    child_ids.append(cid)
    add_dep(parent_3wide, cid)

# ---- 4. Dependency-blocked issues (≥5) ----------------------------------------
blocker_ids = []
for i in range(5):
    bid = add_issue(
        title=f"Blocker issue {i+1}: pipeline dependency gate",
        description=f"Blocker {i+1} for dependency-blocked testing.",
        issue_type="task",
        priority=1,
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )
    blocker_ids.append(bid)

for i, blocker in enumerate(blocker_ids):
    blocked_id = add_issue(
        title=f"Dep-blocked issue {i+1}: waiting on pipeline gate",
        description=f"Blocked by blocker {i+1}.",
        issue_type="task",
        priority=2,
        status="open",
        assignee="dave",
        labels=[SHARED_LABEL],
    )
    add_dep(blocker, blocked_id)

# ---- 5. P1 issues sharing created_at (tie-break corpus) -----------------------
# bd doesn't let us set created_at, but we note the intent in the description
# The smoke test verifies P1 priority issues exist (sort tie-break corpus)
for i in range(15):
    add_issue(
        title=f"P1 sort-tiebreak issue {i+1}: workflow priority tie",
        description="P1 issue for active-column sort tie-break testing (created_at tie-break corpus).",
        issue_type="task",
        priority=1,
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL, "tiebreak"],
    )

# ---- 6. Closed issues (≥75) ---------------------------------------------------
# Block of 12 closed issues (kh54 regression: sort stability with shared updated_at)
for i in range(12):
    add_issue(
        title=f"Closed kh54-tiebreak issue {i+1}: workflow status done",
        description="Closed issue for kh54 sort-stability regression (shared updated_at block).",
        issue_type="task",
        priority=2,
        status="closed",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL, "kh54-tiebreak"],
        comments=["Closed as part of kh54 regression corpus"],
    )

# Block of 12 closed issues sharing closed_at (sort tie-break)
for i in range(12):
    add_issue(
        title=f"Closed closed-at-tiebreak issue {i+1}: pipeline completed",
        description="Closed issue for closed_at sort tie-break testing.",
        issue_type="chore",
        priority=3,
        status="closed",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL, "closed-at-tiebreak"],
    )

# Remaining closed issues to hit ≥75 total
remaining_closed = 75 - 12 - 12
for i in range(remaining_closed):
    keyword = KEYWORDS[i % len(KEYWORDS)]
    add_issue(
        title=f"Closed issue {i+1}: {keyword} task completed",
        description=f"Closed {keyword} issue {i+1} for Done column testing.",
        issue_type="task",
        priority=(i % 4),
        status="closed",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )

# ---- 7. Keyword corpus: 20+ issues per keyword --------------------------------
# workflow — already has some; add to reach 20+
for i in range(25):
    add_issue(
        title=f"Workflow automation task {i+1}: improve workflow scheduling",
        description=f"This task improves the workflow engine. Keyword: workflow. Index: {i+1}.",
        issue_type="task",
        priority=(i % 4),
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )

# pipeline — already has some; add to reach 20+
for i in range(25):
    add_issue(
        title=f"Pipeline stage {i+1}: optimize pipeline throughput",
        description=f"Optimize the pipeline stage {i+1} for maximum throughput. Keyword: pipeline.",
        issue_type="task",
        priority=(i % 4),
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )

# dashboard — already has some; add to reach 20+
for i in range(25):
    add_issue(
        title=f"Dashboard widget {i+1}: add dashboard metrics panel",
        description=f"Add metrics panel {i+1} to the dashboard. Keyword: dashboard.",
        issue_type="feature",
        priority=(i % 4),
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )

# ---- 8. Label shared across 20+ issues (already covered by SHARED_LABEL above)
# Add rare labels to a few issues
for i, rlabel in enumerate(RARE_LABELS):
    add_issue(
        title=f"Rare label issue: {rlabel} investigation",
        description=f"Issue tagged with rare label '{rlabel}' for filter testing.",
        issue_type="spike",
        priority=2,
        status="open",
        assignee=LIGHT_ASSIGNEES[i % len(LIGHT_ASSIGNEES)],
        labels=[SHARED_LABEL, rlabel],
    )

# ---- 9. Assignees with many issues: already covered by heavy assignees above
# Add light assignees with exactly one issue each
for i, assignee in enumerate(LIGHT_ASSIGNEES):
    add_issue(
        title=f"Solo assignee issue for {assignee}: workflow task",
        description=f"Single issue assigned to {assignee} for assignee distribution testing.",
        issue_type="task",
        priority=2,
        status="open",
        assignee=assignee,
        labels=[SHARED_LABEL],
    )

# ---- 10. Fill up to ≥510 issues in the active (Ready) group -------------------
# We need total open/in_progress issues >= 510 for cardinality threshold testing.
# Count how many open issues we have so far.

current_open = sum(1 for iss in issues if iss["status"] in ("open", "in_progress"))
needed = max(0, 515 - current_open)

for i in range(needed):
    keyword = KEYWORDS[i % len(KEYWORDS)]
    add_issue(
        title=f"Scale filler {i+1}: {keyword} maintenance task",
        description=f"Filler issue {i+1} to reach cardinality threshold. Keyword: {keyword}.",
        issue_type="task",
        priority=(i % 4),
        status="open",
        assignee=HEAVY_ASSIGNEES[i % len(HEAVY_ASSIGNEES)],
        labels=[SHARED_LABEL],
    )

# ---- Final stats --------------------------------------------------------------
total = len(issues)
n_open = sum(1 for iss in issues if iss["status"] == "open")
n_in_progress = sum(1 for iss in issues if iss["status"] == "in_progress")
n_closed = sum(1 for iss in issues if iss["status"] == "closed")
n_active = n_open + n_in_progress

print(f"Total issues   : {total}", file=sys.stderr)
print(f"Active (open+ip): {n_active}", file=sys.stderr)
print(f"Closed         : {n_closed}", file=sys.stderr)
print(f"Dependencies   : {len(deps)}", file=sys.stderr)

assert n_closed >= 75, f"need >=75 closed, got {n_closed}"
assert n_active >= 510, f"need >=510 active for cardinality threshold, got {n_active}"

seed = {
    "prefix": PREFIX,
    "issues": issues,
    "dependencies": deps,
}

with open(output_path, "w") as f:
    json.dump(seed, f, indent=2, ensure_ascii=False)
    f.write("\n")

print(f"Written: {output_path}", file=sys.stderr)
print(f"Total issues in output: {len(issues)}", file=sys.stderr)
