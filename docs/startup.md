# Startup and the loading indicator

> Part of the [mars-sim documentation](./README.md).

## What it is

The single line of text — `Mars awaits...` — that the game prints while it gets
itself ready, one dot per completed startup phase, erased just before the TUI
takes the screen.

This doc also records **where startup time actually goes**, because the honest
answer surprised us and it is the thing a future contributor will want to know
before trying to optimize world generation.

## Source

- `loading.go` — the `loader` type: the banner, the dots, and `note`.
- `loading_test.go` — pins the escape sequences and the not-a-terminal case.
- `main.go` — `main()` calls `start`/`step`/`clear` around each phase;
  `setUpGlyphs` routes its warnings through `loader.note`.

## How it works

`main()` marks four phases. Each one calls `step()` when it *finishes*, so a dot
is evidence that something completed, not an animation:

| After | Dot |
| --- | --- |
| flags parsed and `validateConfig` passed | banner, no dots |
| `sim.NewEngine` — the world is carved | 1 |
| engine goroutine running, frontend subscribed | 2 |
| glyph widths measured (`setUpGlyphs`) | 3 |

Then `clear()` erases the line and Bubble Tea opens the alt screen. In
`-headless` the line is cleared too, because `runHeadless` prints its own banner.

Two details carry the design:

**It draws on stderr, not stdout.** The alt screen is about to cover stdout, and
`-headless` writes machine-readable stats there that a redirect should not find
studded with dots. Progress is not output.

**Every `step()` repaints the whole line** (`\r` + banner + dots + `\x1b[K`)
rather than appending one dot. The line is not exclusively ours: the glyph probe
(see [terminal-cell-widths.md](./terminal-cell-widths.md)) parks the cursor at
column 1 and erases the line for each glyph it measures. Repainting from column 1
means the probe can scribble on the line freely — the next `step()` restores it.
`loader.note` exists for the same reason in reverse: the probe's warnings also go
to stderr, and printing them directly would glue them onto the end of the
unterminated loading line.

When stderr is not a terminal the line art is skipped entirely — carriage returns
and erase-line escapes are noise in a log file — but `note` still prints, so a
redirected run loses no information.

## Why it is this way

### Where the time actually goes

Measured on a 4-core Xeon @ 2.80GHz:

| Phase | Cost |
| --- | --- |
| `go run .` compiling 158 packages, cold cache | **~11 s** |
| `go run .`, warm cache | ~0.22 s |
| prebuilt binary, process start to first frame | ~9 ms |
| world generation, default 80×40 | ~0.6 ms |
| world generation, 240×120 | ~3.6 ms |
| world generation, 500×300 | ~16 ms |
| world generation, 1000×600 | ~63 ms |
| glyph probe | 32 terminal round trips; sub-ms locally, 250 ms once if the terminal never answers |

**Compilation is the wait, not the game.** World generation is linear in map area
and never a factor: even a 1000×600 map generates in 63 ms. If startup feels slow,
the fix is `go build -o mars-sim . && ./mars-sim` — compile once, start instantly
— not a faster `generate()`.

That also bounds what this indicator can do. Nothing inside the Go program can
print during the compile that precedes it, so the loading line cannot cover the
part that actually takes time. What it does buy is worth having anyway: after a
long silent compile, the first thing on screen says the game is alive, and the
dots distinguish "wedged in the glyph probe" from "wedged in world generation".

### Dead ends

- **A dot per glyph measured.** The probe is the only startup step with real
  latency (32 sequential write/read round trips — noticeable over SSH), so a
  progress callback into it looked attractive. It does not work: the probe owns
  the terminal line in raw mode and rewrites it from column 1 for every glyph, so
  any dot we drew would be overwritten by the next measurement or would flicker
  against it. Giving the probe its own scratch line and cursor-up/cursor-down
  around it was more terminal choreography than a sub-millisecond step deserves.
- **A spinner on a timer goroutine.** Rejected: a spinner keeps twirling when the
  process wedges, which is exactly the moment you want the display to stop and
  tell you where. A dot per completed phase freezes at the phase that hung.
- **Making `generate()` report progress.** Measured first, then dropped — see the
  table. There is no progress to report.

## Extending it

Adding a phase is two lines: do the work in `main()`, call `load.step()` after
it, and add a row to the table above. Keep `step()` *after* the work, never
before — the dot means "done".

Anything that writes to stderr between `load.start()` and `load.clear()` must go
through `load.note`, or it will land on the end of the loading line.

If a future phase is genuinely slow and owns the terminal, the honest fix is to
give it its own line rather than to interleave with this one.

## Related

- [cli.md](./cli.md) — the flags parsed before the loading line starts.
- [terminal-cell-widths.md](./terminal-cell-widths.md) — the glyph probe, the one
  startup step with real latency.
- [frontend-tui.md](./frontend-tui.md) — what takes the screen once the line is
  erased.
