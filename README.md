# zs-logger-bridge

Small desktop bridge that listens for QSOs from **N1MM Logger+** (UDP XML
broadcast) and **JTDX / WSJT-X** (UDP protocol), normalizes them, and posts
them to a zs-logger event's ingest endpoint
(`POST /api/logsheets/{id}/qso`) so contacts show up live on the event page
without manual export/import.

Built with Go + [Wails v2](https://wails.io) — a single small binary per
platform, no Electron/Chromium bundle.

## Status: unbuilt scaffold

This was generated in an environment without Go, the Wails CLI, or a
system webview available, so **none of this has been compiled or run
yet.** The code is complete and was written carefully against the real
APIs (see "What was verified" below), but you should treat the first local
build as the actual test, not this note.

## Project layout

```
main.go              Wails entry point, window config
app.go                Bound backend (settings, start/stop, activity feed)
internal/config       Load/save settings (JSON in the OS user-config dir)
internal/qso          Normalized QSO type shared by both listeners
internal/n1mm         N1MM UDP/XML listener (<contactinfo> -> QSO)
internal/jtdx         JTDX/WSJT-X UDP listener (QsoLoggedMessage -> QSO),
                      via github.com/k0swe/wsjtx-go
internal/dedupe       Small in-memory TTL cache to avoid re-sending the
                      same QSO twice from one bridge instance
internal/uploader     HTTPS POST client with retry/backoff
frontend/dist         Plain HTML/CSS/JS settings UI (no build step)
.github/workflows     CI: builds Windows + macOS on their native runners
```

## Building it

You need Go 1.21+ and the Wails CLI. The frontend has **no build step** —
`frontend/dist` is committed as plain static files, so you don't need
Node/npm at all.

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd zs-logger-bridge
go mod tidy      # resolves exact dependency versions, writes go.sum
wails doctor     # confirms your platform's webview/build deps are present
wails dev        # run it locally with hot reload, to sanity-check first
wails build      # produces build/bin/zs-logger-bridge(.exe/.app)
```

`wails doctor` will tell you about any missing platform dependency (on
Windows that's usually nothing extra with WebView2 preinstalled by
Windows Update; on macOS it's just Xcode command line tools).

Cross-compiling a working GUI binary from Linux isn't reliable for Wails
apps (they link the platform's native webview), so building for Windows
needs a Windows machine and building for macOS needs a Mac. The included
`.github/workflows/build.yml` does both automatically in CI on push —
push this to a GitHub repo and pull the artifacts from the Actions run
instead of building locally, if that's easier.

## Configuring

Open the app and fill in:

- **Server URL** — e.g. `https://logger.amatir.id`
- **Logsheet ID** — the numeric id of the event
- **Ingest token** — from the key icon / "Renew Token" modal on the
  Logsheets page in the logger. Each logsheet has its own token; renewing
  it invalidates the old one immediately.
- **N1MM port** — must match what you set in N1MM under
  *Config → Configure Ports… → Broadcast Data* for "Contact info". N1MM
  has no fixed default; 12060 is just a common convention.
- **JTDX/WSJT-X port** — must match *Settings → Reporting* in
  JTDX/WSJT-X. Their shared default is 2237.

Settings persist to a JSON file in the OS user-config directory
(`os.UserConfigDir()/zs-logger-bridge/config.json`) and the bridge
auto-starts on next launch if all three connection fields are filled in.

## Manually testing the Laravel endpoint

Before wiring up N1MM/JTDX, you can confirm the backend side works with a
plain curl request (replace the token/id/URL):

```sh
curl -X POST https://logger.amatir.id/api/logsheets/1/qso \
  -H "Authorization: Bearer <ingest_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "callsign": "YB2ABC",
    "frequency": "7.074000",
    "mode": "FT8",
    "rst": "-15",
    "sent_rst": "-08",
    "gridlocator": "OI33",
    "logged_at": "2026-07-01 08:30:00",
    "source": "jtdx"
  }'
```

A 201 with `"success": true` means it worked; a 401 means the token is
wrong; a 422 means validation failed (check field lengths — see the note
below about digital-mode reports).

## Known limitations / follow-ups

- **RST field length.** The logger's `rst`/`sent_rst` columns are
  validated as `max:3`, which fits SSB/CW ("59", "599") but not every
  WSJT-X digital exchange (e.g. `"R-15"` is 4 characters). The bridge
  truncates defensively so uploads never get rejected outright, but if you
  want full-fidelity FT8/FT4 reports, bump `rst`/`sent_rst` to `max:5` in
  `BridgeIngestController::rules()` on the logger side.
- **No system tray icon.** Closing the window hides it (the bridge keeps
  listening) rather than quitting — use the in-app Quit button to actually
  exit. A true tray icon (so it's not in the dock/taskbar at all) is a
  reasonable follow-up via `github.com/getlantern/systray`, but combining
  it with Wails needs care around macOS's main-thread requirement for
  both libraries, so it was left out of this first pass rather than
  shipped unverified.
- **`contactreplace`/`contactdelete` from N1MM are ignored** (logged to
  the activity feed as "ignored in v1" but not acted on) — edits/deletes
  in N1MM won't currently propagate. Straightforward to add later: look up
  the existing `Log` by callsign+band+mode+time and update/delete it.
- **No code signing.** Unsigned builds will trigger Gatekeeper warnings on
  macOS and SmartScreen warnings on Windows. Fine for personal/club use;
  worth signing before wider distribution.

## What was verified vs. what wasn't

Verified against current documentation while writing this:
- The N1MM `<contactinfo>` XML shape and the "tens of Hz" `rxfreq` unit.
- `github.com/k0swe/wsjtx-go/v4`'s actual exported API (`MakeServerGiven`,
  `Server.ListenToWsjtx`, `Server.Shutdown`, the `QsoLoggedMessage` struct
  fields) via its pkg.go.dev docs, so the JTDX listener uses a real,
  maintained protocol implementation instead of hand-rolled binary
  parsing.
- The Laravel side against the actual `zs-logger` codebase: existing
  `logs`/`logsheets` schema, the `Log`/`Logsheet` models, the
  already-existing `ingest_token` column and renewal UI, and the
  validation pattern used by the existing `LogController`.

Not verified (no Go/PHP toolchain in the environment this was written in):
- That the Go module actually compiles — dependency versions in `go.mod`
  are my best current read of what's published, but `go mod tidy` may
  need to adjust them.
- That `wails build` succeeds end-to-end (embed paths, JS bindings, window
  behavior).
- That the Laravel route/middleware/controller are syntactically perfect
  — run `php artisan route:list` and hit the endpoint with the curl
  command above to confirm.

Run the build and the curl test as your first step, before pointing real
N1MM/JTDX traffic at it.
