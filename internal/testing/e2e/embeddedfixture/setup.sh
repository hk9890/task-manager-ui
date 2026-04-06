#!/usr/bin/env sh
set -eu

# setup.sh seeds a deterministic embedded-mode beads repository for integration
# and end-to-end tests. It intentionally avoids interactive behavior.

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <repo-dir> <seed.json>" >&2
  exit 2
fi

repo_dir="$1"
seed_file="$2"

if [ ! -d "$repo_dir" ]; then
  mkdir -p "$repo_dir"
fi

prefix="$(jq -r '.prefix' "$seed_file")"

run_bd() {
  (
    cd "$repo_dir"
    BD_NON_INTERACTIVE=1 bd "$@"
  )
}

if [ ! -d "$repo_dir/.git" ]; then
  git -C "$repo_dir" init >/dev/null
fi

if [ ! -d "$repo_dir/.beads" ]; then
  run_bd init --non-interactive --skip-hooks --skip-agents --prefix "$prefix" >/dev/null
fi

jq -c '.issues[]' "$seed_file" | while IFS= read -r issue; do
  issue_id="$(printf '%s' "$issue" | jq -r '.id')"
  title="$(printf '%s' "$issue" | jq -r '.title')"
  description="$(printf '%s' "$issue" | jq -r '.description')"
  issue_type="$(printf '%s' "$issue" | jq -r '.type')"
  priority="$(printf '%s' "$issue" | jq -r '.priority')"
  status="$(printf '%s' "$issue" | jq -r '.status')"
  assignee="$(printf '%s' "$issue" | jq -r '.assignee')"
  labels_csv="$(printf '%s' "$issue" | jq -r '.labels | join(",")')"

  if ! run_bd show "$issue_id" >/dev/null 2>&1; then
    if [ -n "$labels_csv" ]; then
      run_bd create --id "$issue_id" --title "$title" --description "$description" --type "$issue_type" --priority "$priority" --assignee "$assignee" --labels "$labels_csv" >/dev/null
    else
      run_bd create --id "$issue_id" --title "$title" --description "$description" --type "$issue_type" --priority "$priority" --assignee "$assignee" >/dev/null
    fi

    if [ "$status" = "closed" ]; then
      run_bd close "$issue_id" --reason "fixture seeded closed status" >/dev/null
    elif [ "$status" != "open" ]; then
      run_bd update "$issue_id" --status "$status" >/dev/null
    fi

    printf '%s' "$issue" | jq -r '.comments[]?' | while IFS= read -r comment; do
      run_bd comments add "$issue_id" "$comment" >/dev/null
    done
  fi
done

jq -c '.dependencies[]' "$seed_file" | while IFS= read -r dep; do
  blocker_id="$(printf '%s' "$dep" | jq -r '.blocker_id')"
  blocked_id="$(printf '%s' "$dep" | jq -r '.blocked_id')"
  run_bd dep add "$blocked_id" "$blocker_id" >/dev/null 2>&1 || true
done

echo "fixture-ready:$repo_dir"
