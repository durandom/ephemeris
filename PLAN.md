# ephemeris — plan

Status: **planned, not started** (written 2026-09-11). Nothing here is built
yet. A new session starts implementation from this file.

An *ephemeris* is the astronomer's table of where each body stood at each
point in time. This tool keeps that table for backups: it runs the backup,
measures what is in it, and reports both to the estate telemetry sink, so a
failing or bloating backup is visible within the hour instead of three days
later.

## Why this exists

On **2026-09-08** a Time Machine backup on proteus failed (`rc=2`). Every
later hourly run fell into a full-volume scan that never finished. Nobody
noticed for three days, although the failure was written to
`~/.local/state/tm-cycle/cycle.log` the same afternoon: log files are not
read. The rebuild onto a fresh destination fixed the backup. It did not fix
the blindness.

Full RCA: `~/src/durandom/dotfiles/docs/postmortem-2026-09-10-time-machine.md`.
Exclusion design: `~/src/durandom/dotfiles/docs/tm-spotlight-exclusions.md`.
Fleet alert request: [durandom/orrery#27](https://github.com/durandom/orrery/issues/27).

## Decisions already made

| Decision | Reason |
|---|---|
| Name `ephemeris`, nothing `tm`-shaped | ganymede and Linux hosts will want backup reporting too (#27). Implement Time Machine only; keep its code in one package so a second backend has a place to go. Do not build a backend abstraction before there is a second backend. |
| **Go** | Best OpenTelemetry SDK of the options; `~/src/durandom/token-burn` is the working precedent (`go.opentelemetry.io/otel` v1.44, `otlpmetrichttp`, `CGO_ENABLED=0`, cross-compiled release). |
| Own repo, not dotfiles | Follows orrery decision `2026-08-16-agentsview-producers-are-host-infrastructure`: host-local producers are installed by orrery as pinned, checksummed binaries, not by chezmoi. |
| Installed by orrery | `components/macos-launch-agent-service/index.ts` (`MacosLaunchAgentService`) already downloads a GitHub release, verifies archive + binary sha256, and installs a LaunchAgent. `token-burn` and `progeny` ship this way. |
| Data goes to the **estate** sink `otel` on mimas | Not `spellkave-otel` on triton — that is Spellkave's own instance (orrery D21/D32). |
| **Dashboards, no OpenObserve reports** for now | Reports (PDF/PNG/CSV by email) need a separate `report-server` container (headless Chrome) plus `ZO_SMTP_*`; neither exists on mimas (verified live 2026-09-11). Revisit later. |
| Alerts go to **PagerDuty** | Via `pagerduty-{warning,critical}` OpenObserve destinations. These live on the unmerged orrery branch `alerting` (`~/src/durandom/orrery.alerting`); on `main`, D39 (Better Stack) still applies. Alerts wait for that merge. |
| `mac-exclude` stays bash in v1 | It is the user's interactive CLI and it works. `ephemeris` calls it. Porting the scan is a later step. |

## What it replaces

All in `~/src/durandom/dotfiles`:

| Today | Becomes |
|---|---|
| `bin/executable_tm-cycle.sh` | `ephemeris run` |
| `bin/executable_tm-inventory.py` | `ephemeris inventory` |
| `private_Library/private_LaunchAgents/local.tm-cycle.plist.tmpl` | LaunchAgent installed by orrery |
| `bin/executable_mac-exclude` | stays; called by `ephemeris run` |

Read both scripts before writing code — they carry hard-won details in their
comments (sparse files, `getxattr` via libc, first-run drift suppression,
trailing-slash bug).

## Behaviour to preserve from `tm-cycle.sh`

1. Refresh exclusions first, even when the disk is absent:
   `mac-exclude tm apply-catalog`, then `mac-exclude tm scan ~/src`.
   A failing step does not stop the next one (the script ran without `set -e`).
2. Disk absent (`diskutil info <UUID>` fails) → skip the backup, exit 0.
   **Report this as its own outcome** (`skipped_no_disk`). A laptop that is
   away from its disk is normal; the staleness alert must be able to tell
   "unplugged for two days" from "plugged in and failing".
3. Mount by volume UUID if not mounted. Destination today: volume `tm2`,
   UUID `C4711353-63FC-476E-B57B-AD997762DB96`. Make this config, not code.
4. `tmutil startbackup --block --auto`; its stdout carries `Total copied:`
   and `Avg speed:` — parse them.
5. Always unmount afterwards, so the disk can be unplugged at any time.
   An unmount failure is logged, not fatal.

## New behaviour

- **Never interrupt a running backup.** If `tmutil status -X` reports
  `Running=true`, do not start a second one and do not stop it — report
  `already_running`. Time Machine keeps no partial progress; the 2026-09
  incident lost five recovery attempts to abort-and-retry.
- **Report progress while the backup blocks.** Poll `tmutil status -X`
  (plist output) every 60 s during `startbackup --block` and emit files,
  bytes and percent. This makes the 2026-09 stall signature visible on a
  dashboard: a full-volume scan (`totalFiles` ≈ the volume's inode count)
  whose files-per-minute falls toward zero. Postmortem action item
  "detect the clone-lookup stall".
- **Telemetry never breaks the backup.** Export errors are logged and
  swallowed; flush with a short timeout on exit.
- **Make blindness visible.** `inventory` counts `EPERM`/`EACCES` per area
  and reports it. Without Full Disk Access the walk silently skips TCC-protected
  directories today (Python `os.walk` ignores errors).
- **No local baseline.** `tm-inventory.py` keeps `inventory.json` and
  computes drift itself. `ephemeris` only reports current counts; drift is a
  query in OpenObserve (now vs. 30 days ago).
- Inventory runs monthly. Keep the self-scheduling idea (run when the last
  inventory is older than 30 days) — store only a timestamp, not a baseline.

## Signals (proposal — confirm before building)

- **Trace** per run: root span `ephemeris.run`, one child span per phase
  (`apply_catalog`, `scan`, `inventory`, `mount`, `backup`, `unmount`) with
  `rc` and duration.
- **Log record** per run with the outcome as structured attributes:
  `host`, `outcome` (`success` | `failed` | `skipped_no_disk` | `already_running`),
  `rc`, `duration_s`, `bytes_copied`, `destination`. This is the row the
  dashboard and the alerts query. Events from a short-lived process fit logs
  better than metrics.
- **Log record** per inventory area: `area`, `files`, `bytes`, `eperm`.
- Backup progress samples: metrics or span events — decide in step 1.
- OTel dotted names become underscored at ingest; see the traps in
  `~/src/durandom/orrery/hosts/proteus/AGENTS.md`.
- Never put secrets, tokens or email addresses into attributes.

## Prerequisite in orrery: a buffer on the host

**Problem.** `ephemeris` is a short hourly process. If it exports straight to
`otel.tailc66a3b.ts.net:4318` while proteus is off the tailnet, that run's
data is lost — and the runs most likely to be missing are the failing ones.

**Same gap elsewhere on proteus** (verified 2026-09-11):
- `progeny` exports directly to mimas
  (`hosts/proteus/assets/local.progenyd.plist:13`,
  `PROGENY_OTLP_ENDPOINT=http://otel.tailc66a3b.ts.net:4318`).
- The host-metrics collector (`hosts/proteus/assets/host-metrics/collector.yaml`)
  has **no OTLP receiver** and **no persistent `sending_queue`** on its
  exporters; `file_storage` is only used by the crash-report `filelog`
  receiver.

**Proposed fix (host-wide, not per tool):** give the proteus collector an
OTLP receiver bound to `localhost` and a `file_storage`-backed
`sending_queue` on its exporters. `ephemeris` (and later `progeny`) send to
`localhost:4318`; the collector holds data until mimas is reachable.
This is a separate orrery change and lands before phase 2. **Not yet agreed
with the user** — confirm first. Tracked from the progeny side as
[durandom/progeny#1](https://github.com/durandom/progeny/issues/1).

## Phases

Each phase ends in a working state. Orrery changes follow orrery's apply gate
(root `AGENTS.md`): branch → `pulumi preview` → explicit human approval →
apply. Never `pulumi up` without that approval.

### Phase 1 — this repo

- Go module `github.com/durandom/ephemeris`, layout after `token-burn`
  (`cmd/`, `internal/`).
- `ephemeris run`, `ephemeris inventory`, config file (destination UUID,
  scan roots, OTLP endpoint, inventory areas).
- Port inventory: honour `SkipPaths` from
  `/Library/Preferences/com.apple.TimeMachine.plist` and the xattr
  `com.apple.metadata:com_apple_backup_excludeItem` (lstat, no follow);
  count `st_blocks * 512`, not `st_size` (Docker.raw: 4 TB apparent, 0 real).
- Tests: `tmutil` output parsing (`status -X` plist, `Total copied` lines),
  outcome classification, inventory exclusion logic on a temp tree.
- Before cutover: run `ephemeris inventory` next to `tm-inventory.py` and
  compare totals. They must agree within noise.
- CI + release after `token-burn`'s `.github/workflows/{test,release}.yml`:
  darwin arm64 (+ linux for the future), `checksums.txt`.

### Phase 2 — orrery: install + dashboard

- Host buffer (above), if agreed.
- `hosts/proteus/services/ephemeris.ts` via `MacosLaunchAgentService`,
  pinned release, hourly `StartInterval`.
- Declare the sender (orrery D26, `scripts/render-sender-directory.py`).
- **Dashboard in the same change** — orrery D31 requires it and
  `.githooks/pre-push` enforces it. It answers:
  1. Is the backup running? (last outcome and age, per host)
  2. How long does it take and how much does it copy? (trend)
  3. Is a running backup progressing? (progress samples)
  4. What grows inside the backup? (inventory per area over months, `eperm`)
- **Cutover in one step:** disable dotfiles' `local.tm-cycle` in the same
  apply. Two agents running backups against each other would recreate the
  2026-09 abort-and-retry loop.

### Phase 3 — alerts (after orrery `alerting` merges)

- Last outcome `failed` → warning, immediately. `rc=2` on 2026-09-08 would
  have paged that afternoon.
- No `success` for more than 26 h *while runs are arriving* → critical.
  Closes #27. Check how OpenObserve evaluates a query that returns no rows —
  absence alerts are easy to get silently wrong.
- Inventory drift (≥ 20 % and ≥ 20,000 files per area, the thresholds from
  `tm-inventory.py`) → warning.

### Phase 4 — dotfiles cleanup

- Remove `bin/executable_tm-cycle.sh`, `bin/executable_tm-inventory.py`,
  the `local.tm-cycle` plist template; keep `mac-exclude`.
- Update `docs/tm-spotlight-exclusions.md` and the postmortem action items.

## Separate, independent

- **`mac-exclude tm scan` regression**: 35 s → 407 s per hourly run after the
  root widened to `~/src` and the `SCAN_MIN_FILES=500` guard began counting
  every small candidate with `find | wc -l`. Preferred fix: stop counting at
  500. Lives in dotfiles, can land any time.

## Open questions (ask one at a time)

1. Host buffer in the proteus collector — agree to the proposal above?
2. Logs vs. metrics as the primary signal — confirm the proposal above.
3. Full Disk Access: does `ephemeris` need it at all (`tmutil startbackup`
   probably not; the inventory walk partly)? The binaries are unsigned, and a
   TCC grant for an unsigned/ad-hoc binary is tied to its hash, so every
   release could silently drop it. Measure `eperm` without FDA first.
4. Where the config file lives and who owns it (orrery-rendered vs. local).
5. When to retire the frozen `tm` archive volume (31 backups, May–Sept 2026).
