# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. Use a heredoc for multi-line bodies.
- **Read an issue**: `gh issue view <number> --comments`, filtering comments by `jq` and also fetching labels.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

## ⚠️ Multi-account: switch to the `Ljferrer` account first

This machine has two `gh` accounts on `github.com`: **`Ljferrer`** (personal — owns this repo) and `SQPferrer` (work). The active account is a **global toggle**, so before working here, make sure the right one is active:

```bash
gh auth switch --user Ljferrer   # this repo is personal
```

If the wrong account is active, `gh` will hit the API with the wrong token and you'll get permission errors or wrong results.

Repo auto-detection works fine: `origin` uses the SSH host alias `Ljf.github.com`, but `gh` reads `~/.ssh/config`, resolves it back to `github.com`, and infers `Ljferrer/gastown-tower` from `git remote -v` automatically — so bare commands like `gh issue list` work inside this clone. Passing `-R Ljferrer/gastown-tower` explicitly is still a safe, unambiguous habit and is required when running `gh` from outside the clone.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments`.
