// Scum Lab tool registry.
//
// Adding a tool:
//   1. Create tools/scum-lab/tools/<id>.js
//   2. Export a tool object (see the shape below).
//   3. Import it here and append it to `tools`. Order is the nav order.
//   4. Mention it in docs/scum-lab.md so the next person knows it exists.
//
// Do not invent a second shell, a bundler, or a private copy of the cognition
// config. Read and edit `store.config` from tools/scum-lab/shared/store.js.
// The shell owns the Mars chrome, the nav, and YAML import/export.
//
// Tool shape:
//   {
//     id: "focus",          // hash route, kebab-case, stable
//     title: "Focus Tester", // nav label. One short line. No subtitle.
//     summary: "One sentence for the nav tooltip.",
//     mount(host, ctx) {}   // fill host; return an unmount function
//   }
//
// ctx.store is the shared config. ctx.openYAML("export" | "import" | "vocab")
// opens the shell dialog. A tool that doesn't need either can ignore ctx.

import { focusTool } from "./focus.js";
import { grammarTool } from "./grammar.js";
import { spritesTool } from "./sprites.js";
import { speciesTool } from "./species.js";

export const tools = [focusTool, grammarTool, spritesTool, speciesTool];
