// Sprite Designer: talk to Claude about one SVG until it reads as a map tile,
// then download it. The browser calls the Claude API directly with a key the
// person pastes in; nothing goes through a server of ours, and the key never
// leaves localStorage except in the request's own header.
//
// Claude's SVG is untrusted text. It is only ever shown through <img> (a data
// URL), where scripts don't run, and its source only through textContent and
// textarea values. Never innerHTML it.

const API_URL = "https://api.anthropic.com/v1/messages";
const KEY_STORE = "scum-lab.sprites.key";
const SESSION_STORE = "scum-lab.sprites.session";

// Prices are USD per million tokens, for the running cost readout only.
const MODELS = [
  { id: "claude-opus-5-5", title: "Opus 5.5", input: 4, output: 20, effort: true, fallbacks: true },
  { id: "claude-sonnet-5-5", title: "Sonnet 5.5", input: 2, output: 10, effort: true, fallbacks: true },
  { id: "claude-haiku-4-5", title: "Haiku 4.5", input: 1, output: 5, effort: false, fallbacks: false },
];

// The map's own colors (web/src/map/palette.ts), so the preview shows a
// sprite on the tiles it will actually sit on.
const BACKDROPS = [
  { id: "floor", title: "Floor", color: "#f78765" },
  { id: "rock", title: "Rock", color: "#a8402a" },
  { id: "fog", title: "Fog", color: "#2e0d0b" },
];

// Tile sizes in CSS px: zoomed-out glyph threshold, middle, and the top zoom.
const TILE_SIZES = [16, 32, 64];

// The atlas cell (web/src/map/atlas.ts). A PNG export at this size drops
// straight into the same grid.
const ATLAS_CELL = 128;

// The curated alien glyphs a species can roll (internal/glyphs/glyphs.go),
// offered as suggestions for "Stands in for". Free text is fine too.
const ALIEN_GLYPHS = ["👽", "🦎", "🐍", "🐢", "🦖", "🦕", "🐛", "🪲", "🐜", "🦗", "🦂", "🪱", "🛸", "🦠", "👾", "💀"];

const SYSTEM_PROMPT = `You design map sprites for mars-sim, a colony sim set in caverns under Mars. The map is a grid of square tiles. Today each creature and fixture is drawn as an emoji; your SVGs will replace some of them, starting with alien species.

Every SVG you write must follow these rules:
- Root element <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">, square, no width or height attributes.
- Transparent background. The sprite sits on orange floor (#f78765), dark red rock (#a8402a), or near-black fog (#2e0d0b), so use a dark outline or strong value contrast to stay legible on all three.
- It is drawn as small as 16x16 pixels. Favor a bold silhouette, a few large shapes, strokes no thinner than 4 units, and at most about six colors. Fine detail turns to mush.
- Fill most of the 128 square with the subject (about 8 units of margin), centered, facing the viewer or three-quarter view, like an emoji.
- Fully self-contained: no <script>, <foreignObject>, <image>, external href, @import, web fonts, or <text>. Gradients and filters are allowed but keep them simple.

Reply with one or two sentences about what you drew or changed, then exactly one fenced code block tagged svg containing the complete SVG. Always send the whole file, never a diff.`;

let session = loadSession() || freshSession();
let pending = null; // AbortController while a request is in flight

function freshSession() {
  return {
    name: "",
    standsFor: "👽",
    fieldNotes: "", // a species' field notes pasted from the game's Lore panel
    notesSent: "", // the field notes Claude last saw, so a change gets resent
    model: MODELS[0].id,
    effort: "medium",
    messages: [], // the API conversation, assistant content kept verbatim
    versions: [], // { svg, note, prompt, source: "claude" | "edit" }
    current: -1,
    cost: { input: 0, output: 0, dollars: 0 },
  };
}

function loadSession() {
  try {
    const raw = localStorage.getItem(SESSION_STORE);
    return raw ? { ...freshSession(), ...JSON.parse(raw) } : null;
  } catch {
    return null;
  }
}

function saveSession() {
  try {
    localStorage.setItem(SESSION_STORE, JSON.stringify(session));
  } catch {
    // Private window or a full quota: the session still lives in memory.
  }
}

function loadKey() {
  try { return localStorage.getItem(KEY_STORE) || ""; } catch { return ""; }
}

function saveKey(key) {
  try {
    if (key) localStorage.setItem(KEY_STORE, key);
    else localStorage.removeItem(KEY_STORE);
  } catch {
    // Not remembered; it still works for this page load.
  }
}

export const spritesTool = {
  id: "sprites",
  title: "Sprite Designer",
  summary: "Iterate on an SVG map sprite with Claude, preview it at tile size, and download it.",
  mount(host) {
    const ac = new AbortController();
    host.innerHTML = render();
    wire(host, ac.signal);
    paint(host);
    return () => {
      ac.abort();
      // A request in flight keeps going; its answer lands in the session and
      // shows next time the tab mounts.
    };
  },
};

function render() {
  const models = MODELS.map(model => `<option value="${model.id}">${model.title} ($${model.input} / $${model.output})</option>`).join("");
  const glyphs = ALIEN_GLYPHS.map(glyph => `<option value="${glyph}"></option>`).join("");
  const backdrops = BACKDROPS.map(backdrop => `
    <div class="sprite-row">
      <span class="sprite-row-label">${backdrop.title}</span>
      ${TILE_SIZES.map(size => `
        <span class="sprite-tile" style="background:${backdrop.color};width:${size}px;height:${size}px">
          <img data-tile alt="" width="${size}" height="${size}">
        </span>`).join("")}
      <span class="sprite-tile sprite-emoji" style="background:${backdrop.color}" data-emoji title="The emoji it replaces, at 32px"></span>
    </div>`).join("");

  return `
  <div class="story sprites">
    <section class="tile">
      <p class="step">01 · Key</p>
      <h2>Claude API key</h2>
      <p class="lede">Calls go straight from this page to the Claude API and bill the key's Console account per token. A claude.ai subscription doesn't cover them. The key stays in this browser's storage.</p>
      <div class="form-grid">
        <label class="field wide">API key
          <input type="password" id="sprite-key" autocomplete="off" spellcheck="false" placeholder="sk-ant-…">
        </label>
        <label class="field" title="USD per million input / output tokens">Model
          <select id="sprite-model">${models}</select>
        </label>
        <label class="field">Effort
          <select id="sprite-effort">
            <option value="low">Low: fastest, cheapest</option>
            <option value="medium">Medium</option>
            <option value="high">High: most careful</option>
          </select>
        </label>
      </div>
      <div class="form-actions">
        <button type="button" class="ghost" id="sprite-forget">Forget key</button>
        <span class="hint" id="sprite-cost"></span>
      </div>
    </section>

    <section class="tile">
      <p class="step">02 · Brief</p>
      <h2>What to draw</h2>
      <div class="form-grid">
        <label class="field">Sprite name
          <input type="text" id="sprite-name" placeholder="cave-lizard" spellcheck="false">
        </label>
        <label class="field">Stands in for
          <input type="text" id="sprite-stands" list="sprite-glyphs" spellcheck="false">
          <datalist id="sprite-glyphs">${glyphs}</datalist>
        </label>
      </div>
      <label class="field wide sprite-prompt">
        <span>Field notes <small>(optional) Paste a species' notes from the game's Lore panel.</small></span>
        <textarea id="sprite-notes" rows="3" placeholder="Grelks stand 1.2-1.4 m (3'11&quot;-4'7&quot;) tall, weighing 54-63 kg (119-139 lb). They are skittish around humans; approach with caution. They can be recognized by their gray chitin, 1 eye, 2 arms, 3 legs, and a tail. Get too close and they lash out by biting and thrashing their tails."></textarea>
      </label>
      <label class="field wide sprite-prompt">
        <span id="sprite-prompt-label">Describe it, or add to the field notes</span>
        <textarea id="sprite-prompt" rows="3" placeholder="A squat, six-legged cave lizard with glowing green eyes and a ridge of bony plates."></textarea>
      </label>
      <div class="form-actions">
        <button type="button" class="primary" id="sprite-send">Draw it</button>
        <button type="button" class="ghost" id="sprite-stop" hidden>Stop</button>
        <button type="button" class="ghost" id="sprite-new">New sprite</button>
      </div>
      <p class="note" id="sprite-note"></p>
    </section>

    <section class="tile">
      <p class="step">03 · Preview</p>
      <h2>On the map</h2>
      <div class="sprite-stage">
        <div class="sprite-big" id="sprite-big"><img id="sprite-big-img" alt="Current sprite"><p class="hint" id="sprite-empty">Nothing drawn yet.</p></div>
        <div class="sprite-rows">
          <p class="hint">Tile sizes 16, 32 and 64 px, then the emoji it replaces.</p>
          ${backdrops}
        </div>
      </div>
      <p class="sprite-says" id="sprite-says"></p>
      <ul class="sprite-lint" id="sprite-lint"></ul>
      <div class="sprite-versions" id="sprite-versions"></div>
    </section>

    <section class="tile">
      <p class="step">04 · Export</p>
      <h2>Source and download</h2>
      <p class="hint">Edit by hand if you like. Claude sees your edit with the next message.</p>
      <textarea id="sprite-source" spellcheck="false" class="sprite-source"></textarea>
      <div class="form-actions">
        <button type="button" class="ghost" id="sprite-apply">Use my edit</button>
        <button type="button" class="primary" id="sprite-svg">Download SVG</button>
        <button type="button" class="ghost" id="sprite-png">Download ${ATLAS_CELL}px PNG</button>
        <button type="button" class="ghost" id="sprite-copy">Copy SVG</button>
      </div>
      <p class="note" id="sprite-export-note"></p>
    </section>
  </div>`;
}

function wire(host, signal) {
  const $ = selector => host.querySelector(selector);
  const on = (selector, event, fn) => $(selector).addEventListener(event, fn, { signal });

  $("#sprite-key").value = loadKey();
  $("#sprite-model").value = session.model;
  $("#sprite-effort").value = session.effort;
  $("#sprite-name").value = session.name;
  $("#sprite-stands").value = session.standsFor;
  $("#sprite-notes").value = session.fieldNotes;

  on("#sprite-key", "change", event => saveKey(event.target.value.trim()));
  on("#sprite-forget", "click", () => {
    saveKey("");
    $("#sprite-key").value = "";
  });
  on("#sprite-model", "change", event => {
    session.model = event.target.value;
    saveSession();
    paint(host);
  });
  on("#sprite-effort", "change", event => {
    session.effort = event.target.value;
    saveSession();
  });
  on("#sprite-name", "input", event => {
    session.name = event.target.value;
    saveSession();
  });
  on("#sprite-stands", "input", event => {
    session.standsFor = event.target.value;
    saveSession();
    paint(host);
  });
  on("#sprite-notes", "input", event => {
    session.fieldNotes = event.target.value;
    const species = speciesName(session.fieldNotes);
    if (species && !$("#sprite-name").value.trim()) {
      $("#sprite-name").value = species;
      session.name = species;
    }
    saveSession();
  });
  on("#sprite-send", "click", () => send(host));
  on("#sprite-prompt", "keydown", event => {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) send(host);
  });
  on("#sprite-stop", "click", () => pending?.abort());
  on("#sprite-new", "click", () => {
    if (session.versions.length && !confirm("Start over? The current sprite and its history go away.")) return;
    pending?.abort();
    const keep = { model: session.model, effort: session.effort, standsFor: session.standsFor };
    session = { ...freshSession(), ...keep };
    saveSession();
    $("#sprite-name").value = "";
    $("#sprite-notes").value = "";
    $("#sprite-prompt").value = "";
    note(host, "");
    paint(host);
  });
  on("#sprite-versions", "click", event => {
    const button = event.target.closest("[data-version]");
    if (!button) return;
    session.current = Number(button.dataset.version);
    saveSession();
    paint(host);
  });
  on("#sprite-apply", "click", () => {
    const svg = $("#sprite-source").value.trim();
    if (!svg.startsWith("<svg") && !svg.startsWith("<?xml")) {
      exportNote(host, "That doesn't look like an SVG.", "bad");
      return;
    }
    addVersion({ svg, note: "Hand edit.", prompt: "", source: "edit" });
    exportNote(host, "Saved as a new version.", "ok");
    paint(host);
  });
  on("#sprite-svg", "click", () => {
    const svg = currentSVG();
    if (!svg) return;
    download(new Blob([svg], { type: "image/svg+xml" }), `${fileStem()}.svg`);
  });
  on("#sprite-png", "click", async () => {
    const svg = currentSVG();
    if (!svg) return;
    try {
      download(await rasterize(svg, ATLAS_CELL), `${fileStem()}.png`);
    } catch (error) {
      exportNote(host, error.message, "bad");
    }
  });
  on("#sprite-copy", "click", async () => {
    const svg = currentSVG();
    if (!svg) return;
    try {
      await navigator.clipboard.writeText(svg);
      exportNote(host, "Copied.", "ok");
    } catch {
      exportNote(host, "Clipboard blocked. Select the source and copy it yourself.", "bad");
    }
  });
}

function paint(host) {
  const $ = selector => host.querySelector(selector);
  const version = session.versions[session.current];
  const svg = version?.svg || "";
  const url = svg ? svgURL(svg) : "";

  $("#sprite-big-img").hidden = !svg;
  $("#sprite-empty").hidden = Boolean(svg);
  if (url) $("#sprite-big-img").src = url;
  for (const img of host.querySelectorAll("[data-tile]")) {
    img.hidden = !svg;
    if (url) img.src = url;
  }
  for (const cell of host.querySelectorAll("[data-emoji]")) cell.textContent = session.standsFor || "";

  $("#sprite-says").textContent = version ? version.note : "";
  const lint = $("#sprite-lint");
  lint.replaceChildren(...lintSVG(svg).map(text => {
    const li = document.createElement("li");
    li.textContent = text;
    return li;
  }));

  const versions = $("#sprite-versions");
  versions.replaceChildren(...session.versions.map((v, i) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = i === session.current ? "sprite-version on" : "sprite-version";
    button.dataset.version = String(i);
    button.title = v.source === "edit" ? `v${i + 1}: hand edit` : `v${i + 1}: ${v.prompt}`;
    const img = document.createElement("img");
    img.src = svgURL(v.svg);
    img.alt = "";
    const label = document.createElement("span");
    label.textContent = `v${i + 1}`;
    button.append(img, label);
    return button;
  }));

  const source = $("#sprite-source");
  if (document.activeElement !== source) source.value = svg;

  const busy = pending !== null;
  $("#sprite-send").disabled = busy;
  $("#sprite-send").textContent = busy ? "Drawing…" : session.versions.length ? "Revise it" : "Draw it";
  $("#sprite-stop").hidden = !busy;
  $("#sprite-prompt-label").textContent = session.versions.length ? "What to change" : "Describe it, or add to the field notes";
  $("#sprite-effort").disabled = !modelInfo().effort;

  const cost = session.cost;
  $("#sprite-cost").textContent = cost.input || cost.output
    ? `This sprite so far: ${cost.input.toLocaleString()} in, ${cost.output.toLocaleString()} out, about $${cost.dollars.toFixed(2)}.`
    : "";
}

function modelInfo() {
  return MODELS.find(model => model.id === session.model) || MODELS[0];
}

function currentSVG() {
  return session.versions[session.current]?.svg || "";
}

function addVersion(version) {
  session.versions.push(version);
  session.current = session.versions.length - 1;
  saveSession();
}

async function send(host) {
  if (pending) return;
  const $ = selector => host.querySelector(selector);
  const key = $("#sprite-key").value.trim();
  const prompt = $("#sprite-prompt").value.trim();
  if (!key) return note(host, "Paste an API key first.", "bad");
  const notes = session.fieldNotes.trim();
  const newNotes = notes !== "" && notes !== session.notesSent;
  if (!prompt && !newNotes) {
    const ask = session.versions.length ? "Say what to change." : "Describe the sprite or paste its field notes first.";
    return note(host, ask, "bad");
  }
  saveKey(key);

  const parts = [];
  // If the person picked an older version or edited the source, Claude's last
  // SVG isn't what they're looking at. Show it the one they are.
  const lastClaude = [...session.versions].reverse().find(v => v.source === "claude");
  const shown = session.versions[session.current];
  if (shown && shown !== lastClaude) {
    parts.push(`Work from this version instead of your last one:\n\n\`\`\`svg\n${shown.svg}\n\`\`\``);
  }
  // Field notes go out once, and again only when they change: the
  // conversation already carries them.
  if (newNotes) parts.push(fieldNotesBrief(notes, session.notesSent !== ""));
  if (prompt) parts.push(prompt);
  if (!session.versions.length && session.standsFor) parts.push(`(It replaces the ${session.standsFor} emoji on the map.)`);
  const text = parts.join("\n\n");

  const model = modelInfo();
  const messages = [...session.messages, { role: "user", content: text }];
  const body = { model: model.id, max_tokens: 16000, system: SYSTEM_PROMPT, messages };
  const headers = {
    "content-type": "application/json",
    "x-api-key": key,
    "anthropic-version": "2023-06-01",
    // Required for any call made from a browser page: the API refuses
    // cross-origin requests without it.
    "anthropic-dangerous-direct-browser-access": "true",
  };
  if (model.effort) body.output_config = { effort: session.effort };
  if (model.fallbacks) {
    // A safety-classifier refusal retries on Anthropic's recommended model
    // for that category instead of coming back empty.
    body.fallbacks = "default";
    headers["anthropic-beta"] = "server-side-fallback-2026-07-01";
  }

  pending = new AbortController();
  const started = Date.now();
  const tick = setInterval(() => note(host, `Drawing… ${Math.round((Date.now() - started) / 1000)}s`), 1000);
  note(host, "Drawing…");
  paint(host);

  try {
    const response = await fetch(API_URL, { method: "POST", headers, body: JSON.stringify(body), signal: pending.signal });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(apiError(response.status, data));

    addUsage(model, data.usage);
    if (data.stop_reason === "refusal") throw new Error("Claude declined that request. Try rewording it.");

    // Keep the assistant turn exactly as returned (thinking blocks included):
    // the next request must replay it unchanged.
    session.messages = [...messages, { role: "assistant", content: data.content }];
    const reply = (data.content || []).filter(block => block.type === "text").map(block => block.text).join("\n");
    const svg = extractSVG(reply);
    if (!svg) {
      saveSession();
      const cut = data.stop_reason === "max_tokens" ? " The reply hit the length limit." : "";
      throw new Error(`No SVG in the reply.${cut} ${reply.slice(0, 200)}`.trim());
    }
    if (newNotes) session.notesSent = notes;
    addVersion({ svg, note: noteFrom(reply), prompt: prompt || "From the field notes.", source: "claude" });
    if (mounted(host)) $("#sprite-prompt").value = "";
    note(host, `Done in ${Math.round((Date.now() - started) / 1000)}s.`, "ok");
  } catch (error) {
    note(host, error.name === "AbortError" ? "Stopped." : error.message, "bad");
  } finally {
    clearInterval(tick);
    pending = null;
    if (mounted(host)) paint(host);
  }
}

// The game writes field notes (AlienSpecies.Description in internal/sim/lore.go)
// as prose for the player. Tell Claude which parts are drawable and which
// aren't: every creature fills one tile, so size is build, not scale.
function fieldNotesBrief(notes, changed) {
  const lead = changed ? "The field notes changed. Redraw to match the new ones:" : "Draw the species these field notes describe. They are what the player reads in the game:";
  return `${lead}

<field_notes>
${notes}
</field_notes>

Match every countable feature exactly (eyes, arms, legs, wings, tail) so a player could check the sprite against the notes, and use the stated covering and color. Let the temperament show in the pose and expression, and make the attack visible if a body part delivers it (a tail that thrashes, arms that strangle). Height and weight only tell you the build, stocky or lanky: every creature is drawn filling one tile.`;
}

// The species' plural as the notes name it: "Grelks stand …" or
// "The feared Grelks stand …". Used as a default sprite name.
function speciesName(notes) {
  const match = notes.trim().match(/^(?:The feared\s+)?([A-Za-z][\w'-]*)\s+stand\b/);
  return match ? match[1].toLowerCase() : "";
}

// The shell reuses one host for every tool, so after a tab switch the answer
// may land while another tool owns it.
function mounted(host) {
  return host.isConnected && host.querySelector("#sprite-send") !== null;
}

function apiError(status, data) {
  const message = data?.error?.message || "";
  if (status === 401) return "The API rejected that key.";
  if (status === 429) return `Rate limited. Wait a moment and try again. ${message}`.trim();
  if (status === 529 || status >= 500) return `The API is overloaded or down (${status}). Try again shortly.`;
  return `API error ${status}: ${message || "no details"}`;
}

function addUsage(model, usage) {
  if (!usage) return;
  const input = (usage.input_tokens || 0) + (usage.cache_creation_input_tokens || 0) + (usage.cache_read_input_tokens || 0);
  const output = usage.output_tokens || 0;
  session.cost.input += input;
  session.cost.output += output;
  session.cost.dollars += (input * model.input + output * model.output) / 1e6;
}

function extractSVG(text) {
  const fenced = text.match(/```(?:svg|xml|html)?\s*\n([\s\S]*?)```/);
  const body = fenced ? fenced[1] : text;
  const svg = body.match(/<svg[\s\S]*<\/svg>/);
  return svg ? svg[0].trim() : "";
}

function noteFrom(text) {
  return text.replace(/```[\s\S]*?```/g, "").replace(/<svg[\s\S]*<\/svg>/g, "").trim().slice(0, 400);
}

// Cheap checks for what breaks in the atlas or at 16px. Advice, not a gate.
function lintSVG(svg) {
  if (!svg) return [];
  const warnings = [];
  if (!/viewBox\s*=\s*["']\s*0\s+0\s+(\d+(\.\d+)?)\s+\1\s*["']/.test(svg)) warnings.push("No square viewBox starting at 0 0: the atlas cell may crop or stretch it.");
  if (/<script|on\w+\s*=/i.test(svg)) warnings.push("Has script or event handlers. They won't run in the game, so drop them.");
  if (/<foreignObject/i.test(svg)) warnings.push("Has <foreignObject>, which can block drawing it into the map's canvas.");
  if (/(href|src)\s*=\s*["'](?!#|data:)/i.test(svg) || /@import|url\(\s*["']?(?!#|data:)/i.test(svg)) warnings.push("Points at an external file, which won't load in the atlas.");
  if (/<text/i.test(svg)) warnings.push("Uses <text>, which depends on the player's fonts and is unreadable at 16px.");
  if (svg.length > 20000) warnings.push(`It's ${Math.round(svg.length / 1000)} KB. Detail that size won't survive 16px.`);
  return warnings;
}

function svgURL(svg) {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

function rasterize(svg, size) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = size;
      canvas.height = size;
      canvas.getContext("2d").drawImage(img, 0, 0, size, size);
      canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error("The browser couldn't make a PNG.")), "image/png");
    };
    img.onerror = () => reject(new Error("That SVG doesn't render. Fix the source first."));
    img.src = svgURL(svg);
  });
}

function download(blob, filename) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function fileStem() {
  const stem = session.name.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  return stem || "sprite";
}

function note(host, text, kind = "") {
  const el = host.querySelector("#sprite-note");
  if (!el) return;
  el.textContent = text;
  el.className = kind ? `note ${kind}` : "note";
}

function exportNote(host, text, kind = "") {
  const el = host.querySelector("#sprite-export-note");
  el.textContent = text;
  el.className = kind ? `note ${kind}` : "note";
}
