# nota

Notes and reminders for people who live in the terminal. Works on macOS and Linux.

You are in the middle of something and think "check this tomorrow" or "try that
flag later". Opening a notes app breaks your flow. With nota you type one line
and keep going:

```sh
nota add "bump the ingress-nginx chart after the freeze" --at "monday 10:00"
```

Each note remembers where you wrote it (directory, git repo and branch), and a
small background daemon sends a desktop notification when a reminder is due,
even after you close the terminal.

## Quick start

```sh
curl -fsSL https://raw.githubusercontent.com/dignajar/nota/main/install.sh | sh
nota install-service    # starts the reminder daemon in the background
nota doctor             # checks the setup and sends a test notification
```

The script downloads the latest release for your OS and architecture, verifies
its sha256 checksum and installs `nota` into `~/.local/bin` (override with
`NOTA_BIN_DIR`, or pin a release with `NOTA_VERSION=v0.1.0`). Make sure
`~/.local/bin` is on your `PATH`.

* **Linux:** notifications use `notify-send` (`libnotify-bin` on Debian and
  Ubuntu, `libnotify` on Fedora and Arch).
* **macOS:** works out of the box, using the AppleScript that ships with every Mac.

### Build from source

```sh
git clone https://github.com/dignajar/nota
cd nota
make install            # builds and copies the binary to ~/.local/bin
```

Needs Go 1.27 or newer to build, nothing else at runtime.

## Everyday use

```sh
nota add "rotate the staging database password" --in 2h
nota add "ask platform team about the new node pool" --at "tomorrow 9am"
nota add "idea: alert on PVC usage above 80 percent" -t ideas
nota "quick thought" --in 1h      # shorthand: same as nota add "quick thought"
nota --in 1h "quick thought"      # flags can come first too
nota                              # list open notes
nota here                         # only notes from this repo
nota done 3k                      # mark done, any unique id prefix works
nota snooze 3k 30m                # remind me again in 30 minutes
```

## DevOps examples

### Kubernetes

```sh
# Come back to a flaky rollout after lunch
nota add "check api rollout in prod, 2 pods were restarting" --in 3h -t k8s

# Capture command output together with a title (first line becomes the title)
{ echo "pods not Running in prod"; kubectl get pods -A | grep -v Running; } | nota add -t k8s

# Remember to drain a node outside business hours
nota add "drain ip-10-0-4-21 and replace with the new AMI" --at "friday 18:00" -t k8s

# Review what is waiting for the cluster
nota list -t k8s
```

### Terraform and ArgoCD

```sh
# Park a plan you want to review again with fresh eyes
terraform plan -no-color | tail -n 30 | nota add -t terraform
nota edit 7q            # add your comments in the editor

# Reminder tied to the repo you are standing in
cd ~/src/infra-live
nota add "split the vpc module before adding the second region"
nota here               # later, in the same repo, shows only its notes

# GitOps follow ups
nota add "argocd app prod-payments is OutOfSync, check the ignoreDifferences" --in 1h -t argocd
nota add "bump the helm chart version once the PR is merged" --at "tomorrow 11:00" -t helm
```

### AWS and certificates

```sh
nota add "renew the wildcard certificate" --at "2026-11-03 09:00" -t certs
nota add "delete the unused EBS volumes found in the cost report" --at "next monday" -t aws -t cost
nota add "enable deletion protection on the new RDS instance" --in 1d -t aws
nota add "review IAM access analyzer findings" --at "friday 15:00" -t aws
```

Tags can also be written inline, so `nota add "rotate the signing keys #security"`
adds the tag `security`. Several tags can be given as `-t aws -t cost` or `-t aws,cost`.

### On call and incidents

```sh
nota add "write the postmortem for the 02:00 queue backlog" --at "tomorrow 10:00" -t incident
nota add "ask the vendor for the root cause of the packet loss" --in 4h -t incident
nota add "add an alert for consumer lag above 10k messages" -t incident -t alerts
nota list -t incident          # everything left to close after the incident
nota search lag                # find a note by any word
```

### Docker, CI and Git

```sh
nota add "try the buildx cache mount to speed up the image build" --in 1h -t docker
nota add "the flaky integration test fails only on the arm runner" -t ci
nota add "squash and clean up this branch before review" --in 2h
nota add "ssh into bastion-2 and check the disk usage" --at "tomorrow 8am" -t ops
```

### Use it in scripts

```sh
# Notes as JSON, filtered with jq
nota list --json | jq -r '.[] | select(.git_repo == "infra-live") | .text'

# Create a note from a script and capture its id
id=$(nota add -q "deploy finished, verify dashboards" --in 15m)

# Get a reminder when a long job ends
make deploy && nota add "deploy done, check the error rate" --in 10m
```

## Commands

| Command | What it does |
|---|---|
| `add "text"` | capture a note. Reads from stdin when no text is given. Flags: `--in 2h`, `--at "tomorrow 9am"`, `-t tag`, `-q` |
| `list` (also `ls`) | open notes, soonest reminder first. Flags: `-a` all, `--done`, `--due`, `-t tag`, `--here`, `-n 10`, `--json` |
| `here` | notes captured in the current git repo, or directory outside git |
| `search words` | notes that contain every word, including done ones |
| `show ID` | one note with all its details |
| `done ID...` and `reopen ID...` | change the status |
| `rm ID...` | delete notes |
| `edit ID` | open the file in `$VISUAL` or `$EDITOR` |
| `snooze ID [30m]` | remind again later, one hour by default |
| `remind ID TIME` and `remind ID off` | set or clear a reminder on an existing note |
| `daemon` | run the scheduler in the foreground |
| `install-service` and `uninstall-service` | manage the background service |
| `doctor` | check the setup and send a test notification |
| `path` | print the data directory |

Run `nota help <command>` for the flags of any command.

## Time formats

`--in` takes a duration:

```
30m   2h   1h30m   2 days   an hour   1w
```

`--at` takes a moment:

```
tomorrow            tomorrow 9am         9am tomorrow
3pm                 17:30                tonight (20:00)
eod (17:00)         friday 14:30         next monday
next week           wednesday evening    2026-11-03 09:00
```

A day without a time means 09:00. A time that already passed today means
tomorrow.

## Where notes live

Every note is a plain Markdown file:

```
~/.local/share/nota/notes/k3f9qx.md
```

```markdown
---
id: k3f9qx
created: 2026-10-12T14:30:05+02:00
remind_at: 2026-10-12T17:30:05+02:00
status: open
notified: false
tags: [k8s]
cwd: /home/diego/src/infra-live
git_root: /home/diego/src/infra-live
git_repo: infra-live
git_branch: main
---

check api rollout in prod, 2 pods were restarting
```

There is no database. You can `grep` the folder, edit files by hand, keep them in
a git repo or sync them with Syncthing. Any `.md` file you drop into the folder
becomes a note. The location is `$NOTA_HOME` if set, otherwise
`$XDG_DATA_HOME/nota`, otherwise `~/.local/share/nota` (also on macOS).

## How reminders work

`nota daemon` watches the notes folder and sends a notification when a reminder
is due, then marks the note as notified. It checks a few seconds apart and wakes
exactly on time when the next reminder is sooner. It follows the wall clock, so
reminders stay accurate when a laptop sleeps.

* If many reminders are overdue at once, for example after the machine was off,
  you get one summary notification instead of a flood.
* If a notification fails to send, the reminder stays pending and is retried.

Service details:

* **Linux:** a systemd user unit at `~/.config/systemd/user/nota.service`. View
  logs with `journalctl --user -u nota`. To keep it running after you log out,
  run `loginctl enable-linger $USER`.
* **macOS:** a launchd agent at `~/Library/LaunchAgents/dev.nota.daemon.plist`.
  Logs go to `~/.local/share/nota/daemon.log`.

The service remembers the binary path and data directory from the moment you
installed it. If you move the binary or change `NOTA_HOME`, run
`nota install-service` again.

## Send reminders anywhere

Set `NOTA_NOTIFY_CMD` to replace desktop notifications with any command. It runs
through `sh -c` and receives `$NOTA_TITLE` and `$NOTA_BODY`. This is handy on a
server with no desktop:

```sh
# Push to your phone with ntfy
export NOTA_NOTIFY_CMD='curl -s -d "$NOTA_BODY" ntfy.sh/my-private-topic'

# Post to a Slack incoming webhook
export NOTA_NOTIFY_CMD='curl -s -X POST -H "Content-type: application/json" \
  --data "{\"text\": \"$NOTA_TITLE: $NOTA_BODY\"}" "$SLACK_WEBHOOK_URL"'
```

For the background service, set the variable in the unit (`systemctl --user edit
nota`) or in the plist under `EnvironmentVariables`.

## Environment variables

| Variable | Purpose |
|---|---|
| `NOTA_HOME` | data directory |
| `NOTA_NOTIFY_CMD` | custom notification command |
| `VISUAL`, `EDITOR` | editor used by `nota edit` |
| `NO_COLOR` | turn off colors |

## Troubleshooting

Run `nota doctor` first. It checks the data directory, the notifier, the service
and git, then sends a test notification.

* **No notification from the service on Linux, but `nota doctor` works in your
  terminal.** The user service cannot see your session bus. Run
  `systemctl --user import-environment DBUS_SESSION_BUS_ADDRESS DISPLAY WAYLAND_DISPLAY`
  and then `systemctl --user restart nota`.
* **No notification on macOS.** Open System Settings, then Notifications, and
  allow Script Editor. A Focus mode can also hide them.
* **A note is skipped with a warning.** Its front matter is not valid. Fix it with
  `nota edit ID` or delete the file.

## Development

```sh
make test      # go test -race ./...
make vet       # gofmt and go vet
make cross     # binaries for macOS and Linux, amd64 and arm64, in dist/
```

The project uses only the Go standard library. Layout:

```
main.go
internal/cli         commands, flag parsing, output
internal/store       one Markdown file per note
internal/timeparse   "2h", "tomorrow 9am", "next friday"
internal/scheduler   due logic and the daemon loop
internal/notify      desktop notifications for macOS and Linux
internal/service     systemd user unit and launchd agent
internal/gitctx      directory, repo and branch capture
```

To publish a release, push a tag such as `v0.1.0`. The release workflow builds
the binaries with GoReleaser and attaches them to a GitHub release.

## License

MIT
