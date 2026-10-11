# nota

Capture notes and reminders without leaving the terminal. macOS and Linux.

```sh
nota add "check MongoDB indexes #db"
nota add "try the new rollout command" --in 2h
nota add "look into the flaky test" --at "tomorrow 9am"
kubectl get pods -A | nota add          # capture command output
nota                                    # what's open?
nota here                               # what did I note in this repo?
nota done k3f
```

Every note remembers where you wrote it (directory, git repo and branch), so
"try this command later" still makes sense next week.

## Install

Requires Go 1.22+ to build. No runtime dependencies.

```sh
make install            # builds and copies to ~/.local/bin/nota
nota install-service    # start the reminder daemon in the background
nota doctor             # check everything and send a test notification
```

Make sure `~/.local/bin` is on your `PATH`. On Linux, notifications need
`notify-send` (`libnotify-bin` on Debian/Ubuntu, `libnotify` on Fedora/Arch).
On macOS, `brew install terminal-notifier` gives nicer notifications; without
it nota uses AppleScript.

## Commands

| Command | What it does |
|---|---|
| `add "text" [--in 2h \| --at TIME] [-t tag]` | capture a note; reads stdin when no text is given |
| `list` (`ls`, default) | open notes; `-a` all, `--done`, `--due`, `-t tag`, `--here`, `-n 10`, `--json` |
| `here` | notes captured in the current repo (or directory outside git) |
| `search words...` | notes containing every word, done ones included |
| `show ID` | full note with details |
| `done ID...` / `reopen ID...` | change status |
| `rm ID...` | delete |
| `edit ID` | open the file in `$VISUAL` / `$EDITOR` |
| `snooze ID [30m]` | remind again later (default 1h) |
| `remind ID TIME` / `remind ID off` | set or clear a reminder on an existing note |
| `daemon` | run the scheduler in the foreground |
| `install-service` / `uninstall-service` | manage the background service |
| `doctor` | diagnose setup |
| `path` | print the data directory |

IDs are short (`k3f9qx`) and any unique prefix works: `nota done k3`.

Tags: write `#tag` in the text or pass `-t tag`.

## Times

`--in` takes durations: `30m`, `2h`, `1h30m`, `2 days`, `an hour`, `1w`.

`--at` takes moments: `tomorrow`, `tomorrow 9am`, `9am tomorrow`, `3pm`,
`17:30`, `tonight` (20:00), `eod` (17:00), `friday 14:30`, `next monday`,
`next week`, `wednesday evening`, `2026-10-20`, `2026-10-20 14:00`.

A day without a time means 09:00. A bare time that already passed today means
tomorrow.

## Storage

Plain files, one per note:

```
~/.local/share/nota/notes/k3f9qx.md
```

```markdown
---
id: k3f9qx
created: 2026-10-12T14:30:05+02:00
remind_at: 2026-10-12T16:30:05+02:00
status: open
notified: false
tags: [db]
cwd: /home/diego/src/api
git_root: /home/diego/src/api
git_repo: api
git_branch: main
---

check MongoDB indexes #db
```

The location is `$NOTA_HOME`, else `$XDG_DATA_HOME/nota`, else
`~/.local/share/nota` (also on macOS). Writes are atomic (temp file, fsync,
rename). You can `grep` the folder, edit files by hand, sync it with git or
Syncthing, or drop any `.md` file in and it becomes a note. Unknown front
matter keys are preserved. Unreadable files are skipped with a warning.

## How reminders work

`nota daemon` checks the notes folder every few seconds (it only rereads files
when a cheap fingerprint of names, sizes and mtimes changes) and sleeps
precisely until the next reminder when that is sooner. When a reminder is due
it sends a notification and sets `notified: true` in the file.

It polls instead of setting one long timer on purpose: Go timers use the
monotonic clock, which stops while a Mac is asleep, so a timer set before
closing the lid would fire late. Polling always follows the wall clock.

If several reminders are due at once (the machine was off for a while) you get
one summary notification instead of a flood. If sending fails, the reminder
stays pending and is retried.

The service:

* **Linux**: `~/.config/systemd/user/nota.service`, logs with
  `journalctl --user -u nota`. To keep it running after logout:
  `loginctl enable-linger $USER`.
* **macOS**: `~/Library/LaunchAgents/dev.nota.daemon.plist`, logs in
  `~/.local/share/nota/daemon.log`.

The service is pinned to the binary path and data dir at install time. After
moving the binary or changing `NOTA_HOME`, run `nota install-service` again.

## Custom notifications

Set `NOTA_NOTIFY_CMD` to replace desktop notifications with any command. It
runs with `sh -c` and gets `$NOTA_TITLE` and `$NOTA_BODY`. Useful on headless
servers:

```sh
export NOTA_NOTIFY_CMD='curl -s -d "$NOTA_BODY" ntfy.sh/my-private-topic'
```

For the background service, put the variable in the unit
(`systemctl --user edit nota`) or the plist `EnvironmentVariables`.

## Troubleshooting

* **No notifications on Linux from the service, but `nota doctor` works in a
  terminal**: the user service cannot see your session bus. Run
  `systemctl --user import-environment DBUS_SESSION_BUS_ADDRESS DISPLAY WAYLAND_DISPLAY`
  then `systemctl --user restart nota`.
* **No notifications on macOS**: open System Settings, Notifications, and allow
  "Script Editor" (AppleScript) or "terminal-notifier". Focus modes also hide
  them.
* **Daemon not running**: `nota doctor` shows the service state and the log
  location.

## Development

```sh
make test     # go test -race ./...
make vet
make cross    # dist/nota-{darwin,linux}-{amd64,arm64}
```

Layout:

```
main.go
internal/cli         commands, flag parsing, output
internal/store       one Markdown file per note, front matter codec
internal/timeparse   "2h", "tomorrow 9am", "next friday"
internal/scheduler   due logic and the daemon loop
internal/notify      osascript / terminal-notifier, notify-send / gdbus, custom cmd
internal/service     systemd user unit, launchd agent
internal/gitctx      cwd, repo and branch capture
```

No third party dependencies; everything is the Go standard library.
