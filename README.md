# nota

[![CI](https://github.com/dignajar/nota/actions/workflows/ci.yml/badge.svg)](https://github.com/dignajar/nota/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/dignajar/nota/badge)](https://scorecard.dev/viewer/?uri=github.com/dignajar/nota)
[![Release](https://img.shields.io/github/v/release/dignajar/nota)](https://github.com/dignajar/nota/releases/latest)

Notes and reminders for people who live in the terminal. Works on macOS and Linux.

You are in the middle of something and think "check this tomorrow" or "try that
flag later". Opening a notes app breaks your flow. With nota you type one line
and keep going:

```sh
nota add "bump the ingress-nginx chart after the freeze" --at "monday 10:00"
```

Each note remembers where you wrote it (directory, git repo and branch). When a
reminder is due, you get a regular notification from your system: Notification
Center on macOS, your desktop's notifications on Linux.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/dignajar/nota/main/install.sh | sh
nota install-service    # lets reminders fire in the background
nota doctor             # checks the setup and sends a test notification
```

The script installs the latest release into `~/.local/bin` after verifying its
checksum. Set `NOTA_BIN_DIR` to change the folder, or `NOTA_VERSION=v0.1.0` to
pin a version. On Linux you also need `notify-send` (`libnotify-bin` on Debian
and Ubuntu, `libnotify` on Fedora and Arch).

To build from source you need Go 1.27 or newer: `make install`.

## Everyday use

```sh
nota add "rotate the staging database password" --in 2h
nota add "ask platform team about the new node pool" --at "tomorrow 9am"
nota add "idea: alert on PVC usage above 80 percent" -t ideas
nota "quick thought" --in 1h      # shorthand for nota add
nota                              # list open notes, numbered
nota here                         # only notes from this repo
nota done 2                       # use the number from the list, or an id
nota rm 2
nota snooze 3 30m                 # remind me again in 30 minutes
```

More examples:

```sh
# Capture command output; the first line becomes the title
{ echo "pods not Running in prod"; kubectl get pods -A | grep -v Running; } | nota add -t k8s

# Get a reminder when a long job ends
make deploy && nota add "deploy done, check the error rate" --in 10m

# Notes as JSON
nota list --json | jq -r '.[] | select(.git_repo == "infra-live") | .text'
```

Tags can be written inline (`#security`) or with `-t aws -t cost`.

## Commands

| Command | What it does |
|---|---|
| `add "text"` | capture a note (reads stdin when no text). Flags: `--in 2h`, `--at "tomorrow 9am"`, `-t tag`, `-q` |
| `list` | open notes, numbered, soonest reminder first. Flags: `-a`, `--done`, `--due`, `-t tag`, `--here`, `-n 10`, `--json` |
| `here` | notes from the current git repo or directory |
| `search words` | notes containing every word, including done ones |
| `show`, `edit`, `append` | read a note, open it in `$EDITOR`, or add a line to it |
| `done`, `reopen`, `rm` | change the status or delete (accept several) |
| `snooze`, `remind` | remind again later, or set or clear a reminder |
| `install-service`, `uninstall-service`, `daemon` | the background reminder service |
| `doctor`, `path` | check the setup, print the data folder |
| `mcp` | MCP server for Claude Code, Cursor and other agents |

Everywhere a note is expected you can use its number from `nota list` or its id
(a unique prefix is enough). Run `nota help <command>` for the flags.

### Time formats

`--in` takes a duration: `30m`, `2h`, `1h30m`, `2 days`, `1w`.

`--at` takes a moment: `tomorrow 9am`, `17:30`, `tonight`, `eod`, `friday 14:30`,
`next monday`, `2026-11-03 09:00`. A day without a time means 09:00.

## Use it from Claude Code, Cursor and other agents

`nota mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server,
so an agent can take notes and set reminders for you.

```sh
claude mcp add nota -- nota mcp     # Claude Code
```

For Cursor, add `{ "mcpServers": { "nota": { "command": "nota", "args": ["mcp"] } } }`
to `~/.cursor/mcp.json`. Then ask "remind me in 2h to check the deploy".

## Where notes live

Every note is a plain Markdown file in `~/.local/share/nota/notes/` (or
`$NOTA_HOME`):

```markdown
---
id: k3f9qx
created: 2026-10-12T14:30:05+02:00
remind_at: 2026-10-12T17:30:05+02:00
status: open
tags: [k8s]
git_repo: infra-live
git_branch: main
---

check api rollout in prod, 2 pods were restarting
```

There is no database. You can `grep` the folder, edit files by hand or sync them
with git or Syncthing.

## Reminders and the background service

`nota install-service` runs a small service that checks for due reminders and
sends the notification. It uses a systemd user unit on Linux and a launchd agent
on macOS. If you move the binary or change `NOTA_HOME`, run it again.

To send reminders somewhere else, set `NOTA_NOTIFY_CMD`. It runs through
`sh -c` with `$NOTA_TITLE` and `$NOTA_BODY`, which is handy on a server:

```sh
export NOTA_NOTIFY_CMD='curl -s -d "$NOTA_BODY" ntfy.sh/my-private-topic'
```

## Troubleshooting

Run `nota doctor` first.

* **Linux: no notification from the service, but `nota doctor` works in your
  terminal.** Run `systemctl --user import-environment DBUS_SESSION_BUS_ADDRESS DISPLAY WAYLAND_DISPLAY`
  and then `systemctl --user restart nota`. Logs: `journalctl --user -u nota`.
* **macOS: no notification.** In System Settings, Notifications, allow Script
  Editor. A Focus mode can also hide them.
* **A note is skipped with a warning.** Its front matter is not valid. Fix it
  with `nota edit ID` or delete the file.

## Development

```sh
make test      # go test -race ./...
make vet       # gofmt and go vet
make release   # start a release (BUMP=auto|patch|minor|major)
```

Only the Go standard library is used. Every merge to `main` publishes a release;
the version comes from the conventional commit titles.

## License

MIT
