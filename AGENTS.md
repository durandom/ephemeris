# AGENTS.md

`ephemeris` runs host backups and reports their outcome and contents to the
estate OpenTelemetry sink (OpenObserve on mimas), so a failing or bloating
backup is visible on a dashboard and pages via PagerDuty.

**Start with `PLAN.md`.** It holds the decisions already made, the behaviour
to preserve from the scripts this replaces, the phases, and the open
questions. Ask the open questions one at a time before building past them.

Related repos:

- `~/src/durandom/dotfiles` — the scripts being replaced, `mac-exclude`, the
  2026-09-10 postmortem.
- `~/src/durandom/orrery` — installs this binary on proteus, owns the sink,
  dashboards and alerts. Follow its apply gate: never `pulumi up` without
  explicit human approval.
- `~/src/durandom/token-burn` — Go + OTel + release precedent.

Rules:

- Telemetry must never make a backup fail or stop.
- Never interrupt a running Time Machine backup.
- No secrets or email addresses in telemetry attributes.
- Files are written in English.
