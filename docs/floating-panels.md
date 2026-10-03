# Floating panels

> Part of the [mars-sim documentation](./README.md).

## What it is

In the browser, any side-panel tab can be **popped out** into a window over
the map, so two or more tabs can be watched at once: the Market beside a
chart, the Inspector beside the Roster. The side panel keeps showing one
docked tab; popped-out tabs float above the map, each dragged by its title
row and resized from its bottom-right corner. Which tabs float, and where,
is remembered in this browser.

## Source

- [`web/src/game.svelte.ts`](../web/src/game.svelte.ts) — the state
  (`ui.floats`, a `FloatWin` per window, back to front) and the actions:
  `popOut`, `dock`, `closeFloat`, `raiseFloat`, `placeFloat`, `isFloating`.
  `setPanel` and `inspect` raise a floating tab instead of docking it.
- [`web/src/ui/FloatingPanels.svelte`](../web/src/ui/FloatingPanels.svelte) —
  the windows: drawing, dragging, resizing, keeping them on screen.
- [`web/src/ui/PanelHead.svelte`](../web/src/ui/PanelHead.svelte) — a tab's
  title row with its buttons, shared by the docked panel and the windows.
- [`web/src/ui/tabs.ts`](../web/src/ui/tabs.ts) — the tab list, shared by
  `SidePanel` and `FloatingPanels`.
- [`web/src/ui/SidePanel.svelte`](../web/src/ui/SidePanel.svelte) — the ⧉
  (pop out) button, and the tab strip's dashed outline on a floating tab.

## How it works

- **Pop out:** ⧉ in the docked tab's title row. The tab leaves the side
  panel (which closes) and opens in a window left of it. A new window takes
  the first slot of a small cascade that no window still sits in exactly, so
  it never lands squarely on another.
- **A window's title row** names the tab and has ⇥ (dock: back into the side
  panel, as its open tab, replacing what was docked) and × (close). Drag the
  row to move the window; drag the corner grip to resize it (at least
  260×160).
- **Stacking:** a pointer press anywhere in a window raises it. The windows
  are drawn in the tab strip's order and stacked by `z-index` from their
  place in `ui.floats`, so raising one never moves its DOM node: moving it
  mid-press would drop the pointer capture of the very drag that raised it.
- **The tab strip** outlines a floating tab with a dashed accent border. Its
  button raises the window instead of docking the tab; the window's ⇥ docks
  it. Every route that opens a tab goes through `setPanel` or `inspect`, so a
  click on the map raises a floating inspector. The log ticker hides while
  the Log is open either way.
- **Remembered:** `ui.floats` is saved to `localStorage` as
  `mars-sim.floats` at the end of each drag and on every pop out, dock, close
  and raise (not on every pointer move). It survives reloads and new games.
  An id that no longer names a tab is ignored.
- **On screen:** a window is drawn pulled back inside the viewport (at least
  80 px of its title row showing, never taller or wider than the viewport),
  but its stored place is left alone, so it returns where it was when the
  window grows again.
- **Phones** (under 700 px) do not offer ⧉: the side panel is a bottom sheet
  there and there is no map left to float over.

## Why it is this way

- **A tab is docked or floating, never both.** Mounting one tab twice would
  be cheap (topics are reference-counted), but some tabs own page-wide state
  the two copies would fight over: Dig and Zones arm a map tool, Jobs lays a
  highlight on the map and clears it when it unmounts. One instance per tab
  keeps every tab's lifetime what it was. The cost is that Charts shows one
  view at a time, even floating: Perf and Population side by side would need
  `ui.chartView` per instance.
- **The selection follows what is open, docked or floating.** Leaving the
  inspector used to mean "the side panel shows another tab". Now it is
  "neither the inspector, nor the roster, nor the tab the selection came from
  is open anywhere" (`dropHiddenSelection`). So a floating inspector keeps its
  subject and the map its marker while you page through other tabs, and
  closing its window drops them. One quirk is kept on purpose: closing the
  side panel after a click on the map (no `inspectFrom`) keeps the marker, as
  it always did.
- **Every tab got a title row.** The pop-out button needed a home in the
  tab's own body, not the strip (a strip button would read as a tab). The row
  also names the window once it floats, so Dig, Ships and Zones dropped
  their own `<h2>` title, which would now say it twice.
- **Our own drag and resize, not CSS `resize`.** The CSS grip cannot be
  styled consistently, reports its size only through a `ResizeObserver`, and
  has no "end of drag" to save on. One pointer-capture helper does both move
  and resize, and saves once on release. Charts resize with the window:
  `Chart.svelte` already follows its container with a `ResizeObserver`.
- **Escape still closes only the docked panel.** It is a habit for the side
  panel; a window you arranged on purpose should not vanish with it.

## Extending it

- A new tab is one entry in `tabs.ts`; it can pop out with no more work.
- A tab that should be openable twice (a second Charts view) needs its
  per-tab state in `ui` (like `ui.chartView`) moved to a per-window prop, and
  `FloatWin` keyed by something other than the tab id.
- Anything that asks "is tab X open?" should use `ui.panel === X ||
  isFloating(X)`, as the log ticker does, not `ui.panel` alone.

## Related

- [frontend-web.md](./frontend-web.md) — the side panel and every tab.
- [browser-frontend.md](./browser-frontend.md) — the browser build's plan.
