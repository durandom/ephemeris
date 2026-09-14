# ephemeris

Runs host backups, measures what landed in them, and reports both to the
estate OpenTelemetry sink. A failing or bloating backup should be visible
within the hour.

v1 implements **Time Machine only**. The name is backend-agnostic on purpose
(`ganymede` and Linux hosts will want the same reporting later).

This replaces `tm-cycle.sh` and `tm-inventory.py` in `dotfiles`. `mac-exclude`
stays a bash CLI; `ephemeris run` calls it.

**Do not run `ephemeris run` on proteus until orrery cutover.** Two agents
starting backups against each other recreates the 2026-09 abort-and-retry
loop. `ephemeris inventory` is read-only (it walks the source, it does not
talk to Time Machine) and can be compared against `tm-inventory.py` before
cutover.

## Commands

```
ephemeris run            # exclusions, optional monthly inventory, mount-backup-unmount
ephemeris inventory      # count files/bytes/eperm per area
ephemeris version
```

`--config path` overrides `~/.config/ephemeris/config.toml`. If that file is
missing, built-in defaults matching proteus (`tm2`) are used. Defaults are
not written to disk.

## Behaviour

- Refresh exclusions first, even when the disk is unplugged. A failing
  `mac-exclude` step does not stop the next one.
- Disk absent → outcome `skipped_no_disk`, exit 0.
- `tmutil status` reports `Running=true` → outcome `already_running`. Never
  start a second backup, never stop the one that is running.
- `tmutil startbackup --block --auto`, parse `Total copied`. Always unmount
  afterwards; unmount failure is logged, not fatal.
- Poll `tmutil status -X` every 60s while the backup blocks. Progress is a
  log record plus a span event (`files`, `bytes`, `percent` / `raw_percent`).
  `bytes` is not progress — Time Machine clonefile can freeze that counter
  for hours.
- Telemetry never fails the backup. Export errors are swallowed; flush has a
  short timeout on exit.
- Inventory honours `SkipPaths` and the backup-exclude xattr on every file
  and directory, counts `st_blocks * 512` (not apparent size), and reports
  `eperm` per area. Its first total can be lower than `tm-inventory.py`:
  that legacy script incorrectly counted individually xattr-excluded files.
  No local baseline; drift is a query in OpenObserve. A timestamp in
  `~/.local/state/ephemeris/last_inventory` schedules the monthly walk.

Outcomes: `success` | `failed` | `skipped_no_disk` | `already_running`.

## Telemetry

Primary signal is **logs**, not metrics (this is a short-lived hourly
process). Attribute names use underscores (`duration_s`, `bytes_copied`)
because OpenObserve turns dotted names into underscores at ingest.

| Record           | Attributes |
|------------------|------------|
| `ephemeris.run`  | `host`, `outcome`, `rc`, `duration_s`, `bytes_copied`, `destination` |
| `ephemeris.inventory` | `host`, `area`, `files`, `bytes`, `eperm` |
| `ephemeris.progress` | `host`, `phase`, `files`, `total_files`, `bytes`, `percent`, `raw_percent`, `time_remaining_s`, `destination` |

Traces: root span `ephemeris.run`, child spans `apply_catalog`, `scan`,
`inventory`, `mount`, `backup`, `unmount` with `rc`.

OTLP endpoint comes from the config file. Telemetry errors are swallowed and
written as a generic `telemetry export failed` entry to the bounded local
`cycle.log`; endpoints and exporter errors are intentionally not copied there.

No secrets, tokens, or email addresses in attributes.

## Build

```
go test ./...
go build -o bin/ephemeris ./cmd/ephemeris
```

Release artifacts (tag `v*`): unsigned darwin/linux amd64/arm64,
`checksums.txt`, `CGO_ENABLED=0`. Installed later by orrery
`MacosLaunchAgentService`.

A maintainer can additionally create a local Developer-ID-signed darwin/arm64
tarball without putting Apple credentials in GitHub Actions. It uses a
hardened, timestamped raw binary and verifies its signature and Gatekeeper
assessment. Raw binaries cannot be stapled; notarization and package work are
deliberately deferred. See [docs/RELEASING.md](docs/RELEASING.md).
