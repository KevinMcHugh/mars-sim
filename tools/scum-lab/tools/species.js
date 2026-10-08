// Species Catalog: every species in Creature Lab, with its sprites. It reads
// the lab's public API (GET /api/species, /api/species/{id} and its
// candidates), which needs no key and allows any origin, so this works from
// GitHub Pages and from localhost alike. It never writes; drawing and
// accepting happen in Creature Lab or over its MCP tools. See
// docs/creature-lab.md.
//
// Everything the lab returns is shown through textContent, and sprites only
// through <img>, where an SVG's scripts don't run. Every URL it returns goes
// through safeURL first: the lab is whatever server ?lab= names, and a
// javascript: link on this origin could read the Sprite Designer's API key.

// The deployed lab. A different one (a local `make dev`) can be typed in;
// the choice is remembered in this browser.
const DEFAULT_LAB = "https://creature-lab-b2mxg.sprites.app";
const LAB_STORE = "scum-lab.species.lab";

// The map's own colors (web/src/map/palette.ts), as the Sprite Designer uses.
const BACKDROPS = [
  { title: "Floor", color: "#f78765" },
  { title: "Rock", color: "#a8402a" },
  { title: "Fog", color: "#2e0d0b" },
];
const TILE_SIZES = [16, 32, 64];

let species = []; // the last list fetched, newest first
let open = ""; // the id of the species whose detail is showing
let filter = "";

// loadLab is the lab to read: a ?lab= in the page URL (so a link can point
// at a local lab), else the one last loaded here, else the deployed one.
function loadLab() {
  const fromURL = new URLSearchParams(location.search).get("lab");
  if (fromURL) return fromURL.replace(/\/+$/, "");
  try { return localStorage.getItem(LAB_STORE) || DEFAULT_LAB; } catch { return DEFAULT_LAB; }
}

function saveLab(url) {
  try {
    if (url && url !== DEFAULT_LAB) localStorage.setItem(LAB_STORE, url);
    else localStorage.removeItem(LAB_STORE);
  } catch {
    // Not remembered; it still works for this page load.
  }
}

export const speciesTool = {
  id: "species",
  title: "Species Catalog",
  summary: "Every species in Creature Lab, with its sprites, read from the lab's public API.",
  mount(host) {
    const ac = new AbortController();
    host.append(el("div", { className: "story species" },
      el("section", { className: "tile" },
        el("p", { className: "step", text: "01 · Source" }),
        el("h2", { text: "Creature Lab" }),
        el("p", { className: "lede", text: "Species are rolled by the game's own roster code and kept, with a sprite for every form of their life, in Creature Lab. This tab only reads them; draw and accept sprites in the lab or over its MCP tools." }),
        el("form", { className: "species-source", id: "species-form" },
          el("label", { className: "field wide", text: "Lab URL" },
            el("input", { type: "url", id: "species-lab", value: loadLab(), spellcheck: false })),
          el("button", { type: "submit", className: "primary", text: "Load" })),
        el("p", { className: "note", id: "species-status" })),
      el("section", { className: "tile" },
        el("div", { className: "species-head" },
          el("div", {},
            el("p", { className: "step", text: "02 · Catalog" }),
            el("h2", { id: "species-count", text: "Species" })),
          el("input", { type: "search", id: "species-filter", placeholder: "Filter species", value: filter })),
        el("div", { className: "species-grid", id: "species-grid" })),
      el("section", { className: "tile", id: "species-detail", hidden: true })));

    const $ = selector => host.querySelector(selector);
    $("#species-form").addEventListener("submit", event => {
      event.preventDefault();
      const url = $("#species-lab").value.trim().replace(/\/+$/, "");
      saveLab(url);
      load(host, url, ac.signal);
    }, { signal: ac.signal });
    $("#species-filter").addEventListener("input", event => {
      filter = event.target.value;
      paintGrid(host, ac.signal);
    }, { signal: ac.signal });

    load(host, loadLab(), ac.signal);
    return () => ac.abort();
  },
};

async function load(host, lab, signal) {
  const status = host.querySelector("#species-status");
  status.textContent = "Loading…";
  try {
    const body = await getJSON(`${lab}/api/species`, signal);
    species = body.species || [];
    status.textContent = `Loaded from ${lab}.`;
    paintGrid(host, signal);
    if (open && species.some(s => s.id === open)) showDetail(host, lab, open, signal);
    else hideDetail(host);
  } catch (error) {
    if (signal.aborted) return;
    species = [];
    paintGrid(host, signal);
    hideDetail(host);
    status.textContent = `Couldn't load species from ${lab}: ${error.message}`;
  }
}

function paintGrid(host, signal) {
  const needle = filter.trim().toLowerCase();
  const shown = species.filter(s => !needle ||
    [s.plural, s.singular, s.scientificName, s.temperament].some(v => (v || "").toLowerCase().includes(needle)));
  const complete = species.filter(s => s.complete).length;
  host.querySelector("#species-count").textContent =
    `${species.length} species · ${complete} with every sprite`;

  const grid = host.querySelector("#species-grid");
  if (!shown.length) {
    grid.replaceChildren(el("p", { className: "hint", text: species.length ? "Nothing matches the filter." : "No species yet." }));
    return;
  }
  grid.replaceChildren(...shown.map(s => {
    const thumbs = el("div", { className: "species-thumbs" });
    if (s.spriteUrls?.length) {
      for (const url of s.spriteUrls) thumbs.append(tile(url, 40, BACKDROPS[0].color));
    } else {
      thumbs.append(el("span", { className: "sprite-tile species-emoji", style: `background:${BACKDROPS[0].color}`, text: s.emoji || "?" }));
    }
    const card = el("button", { type: "button", className: s.id === open ? "species-card on" : "species-card" },
      thumbs,
      el("strong", { text: `${s.emoji ? s.emoji + " " : ""}${s.plural}` }),
      el("em", { className: "species-sci", text: s.scientificName }),
      el("span", { className: "species-tags" },
        el("span", { className: `pill species-${s.temperament}`, text: s.temperament }),
        s.apex ? el("span", { className: "pill species-hostile", text: "apex" }) : null,
        el("span", { className: "pill", text: `${s.acceptedCount}/${s.formCount} sprite${s.formCount === 1 ? "" : "s"}` })),
      el("span", { className: "species-seed", text: `seed ${s.seed}` }));
    card.addEventListener("click", () => {
      open = s.id;
      paintGrid(host, signal);
      showDetail(host, currentLab(host), s.id, signal);
    }, { signal });
    return card;
  }));
}

function currentLab(host) {
  return host.querySelector("#species-lab").value.trim().replace(/\/+$/, "");
}

function hideDetail(host) {
  const detail = host.querySelector("#species-detail");
  detail.hidden = true;
  detail.replaceChildren();
}

async function showDetail(host, lab, id, signal) {
  const detail = host.querySelector("#species-detail");
  detail.hidden = false;
  detail.replaceChildren(el("p", { className: "note", text: "Loading…" }));
  let sp, candidates;
  try {
    [sp, { candidates }] = await Promise.all([
      getJSON(`${lab}/api/species/${encodeURIComponent(id)}`, signal),
      getJSON(`${lab}/api/species/${encodeURIComponent(id)}/candidates`, signal),
    ]);
  } catch (error) {
    if (!signal.aborted && open === id) detail.replaceChildren(el("p", { className: "note", text: `Couldn't load it: ${error.message}` }));
    return;
  }
  if (open !== id) return; // another card was clicked while this loaded

  const t = sp.traits;
  const body = `${t.eyes} eyes, ${t.arms} arms, ${t.legs} legs${t.tail ? ", a tail" : ""}${t.wings ? ", wings" : ""}`;
  const facts = el("dl", { className: "species-facts" });
  const fact = (label, value) => { if (value) facts.append(el("dt", { text: label }), el("dd", { text: value })); };
  fact("Body", body);
  fact("Covering", `${t.color} ${t.skin}, ${t.pattern}`);
  fact("Size", `${t.heightCM[0]}–${t.heightCM[1]} cm, ${t.weightKG[0]}–${t.weightKG[1]} kg`);
  fact("Features", (t.features || []).join(", "));
  fact("Attacks", (t.attacks || []).join(", "));
  fact("Lab notes", sp.notes);
  fact("Rolled by", `seed ${sp.seed} on ${sp.generatorRev}`);

  detail.replaceChildren(
    el("div", { className: "species-head" },
      el("div", {},
        el("p", { className: "step", text: "03 · Species" }),
        el("h2", { text: `${t.emoji ? t.emoji + " " : ""}${t.plural}` }),
        el("em", { className: "species-sci", text: t.scientificName })),
      el("a", { className: "species-link", href: safeURL(sp.url), target: "_blank", rel: "noopener", text: "Open in Creature Lab ↗" })),
    el("p", { className: "lede", text: t.description }),
    facts,
    ...sp.forms.map(form => formSection(form, candidates.filter(c => c.form === form.index))));
}

function formSection(form, candidates) {
  const what = form.inert
    ? "an inert casing"
    : `${form.arms} arms, ${form.legs} legs${form.tail ? ", a tail" : ""}${form.wings ? ", wings" : ""}`;
  const size = form.sizePct !== 100 ? `, ${form.sizePct}% of adult size` : "";
  const section = el("div", { className: "species-form" },
    el("h3", { text: `Form ${form.index} · ${form.name}` }),
    el("p", { className: "hint", text: `${form.label}: ${what}${size}${form.lays ? "; lays the next generation" : ""}.` }));

  if (form.acceptedSvgUrl) {
    const rows = el("div", { className: "sprite-rows" });
    for (const backdrop of BACKDROPS) {
      rows.append(el("div", { className: "sprite-row" },
        el("span", { className: "sprite-row-label", text: backdrop.title }),
        ...TILE_SIZES.map(size => tile(form.acceptedSvgUrl, size, backdrop.color))));
    }
    section.append(rows);
  } else {
    section.append(el("p", { className: "hint", text: "No accepted sprite yet." }));
  }

  if (candidates.length) {
    section.append(el("div", { className: "species-candidates" }, ...candidates.map(c =>
      el("a", { className: c.accepted ? "species-candidate on" : "species-candidate", href: safeURL(c.svgUrl), target: "_blank", rel: "noopener",
        title: [c.accepted ? "accepted" : "candidate", c.author, c.note].filter(Boolean).join(" · ") },
        tile(c.svgUrl, 48, BACKDROPS[0].color),
        el("span", { text: c.accepted ? "accepted" : new Date(c.createdAt).toLocaleDateString() })))));
  }
  return section;
}

function tile(url, size, color) {
  return el("span", { className: "sprite-tile", style: `background:${color};width:${size}px;height:${size}px` },
    el("img", { src: safeURL(url), alt: "", width: size, height: size, loading: "lazy" }));
}

// safeURL passes an absolute http(s) URL through and turns anything else
// (javascript:, data:, a relative path) into an inert "#".
function safeURL(raw) {
  try {
    const url = new URL(raw);
    return url.protocol === "https:" || url.protocol === "http:" ? url.href : "#";
  } catch {
    return "#";
  }
}

async function getJSON(url, signal) {
  const response = await fetch(url, { signal });
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try { message = (await response.json()).error || message; } catch { /* not JSON */ }
    throw new Error(message);
  }
  return response.json();
}

// el builds an element. text goes in through textContent, never as HTML.
function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props)) {
    if (key === "text") node.textContent = value;
    else if (key === "style") node.setAttribute("style", value);
    else if (key === "hidden" || key === "spellcheck") node[key] = value;
    else if (key in node) node[key] = value;
    else node.setAttribute(key, value);
  }
  node.append(...children.filter(child => child !== null && child !== undefined));
  return node;
}
