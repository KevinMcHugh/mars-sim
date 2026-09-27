import { tools } from "./tools/registry.js";
import { store, cloneConfig } from "./shared/store.js";
import { dumpYAML, parseVocabulary, parseYAML } from "./shared/yaml.js";
import { bootSim } from "./shared/sim.js";

const host = document.querySelector("#tool");
const nav = document.querySelector("#nav");
const modal = document.querySelector("#modal");
const modalTitle = document.querySelector("#modal-title");
const modalText = document.querySelector("#modal-text");
const modalNote = document.querySelector("#modal-note");

let current = null;
let modalMode = "export";

function toolFromHash() {
  const id = location.hash.replace("#", "");
  return tools.find(tool => tool.id === id) || tools[0];
}

function renderNav(active) {
  nav.replaceChildren();
  for (const tool of tools) {
    const button = document.createElement("a");
    button.href = `#${tool.id}`;
    button.className = tool.id === active.id ? "nav-link on" : "nav-link";
    button.title = tool.summary || "";
    button.textContent = tool.title;
    nav.append(button);
  }
}

function show(tool) {
  if (current?.unmount) current.unmount();
  host.replaceChildren();
  const unmount = tool.mount(host, {
    store,
    openYAML,
  }) || (() => {});
  current = { id: tool.id, unmount };
  renderNav(tool);
  document.title = `Scum Lab — ${tool.title}`;
  if (location.hash !== `#${tool.id}`) history.replaceState(null, "", `#${tool.id}`);
}

function openYAML(mode) {
  modalMode = mode;
  modalNote.textContent = "";
  if (mode === "export") {
    modalTitle.textContent = "Export cognition.yaml";
    modalText.value = dumpYAML(store.config);
  } else if (mode === "vocab") {
    modalTitle.textContent = "Import vocabulary JSON";
    modalText.value = "";
    modalNote.textContent = "Paste the output of go run . -print-cognition-vocab.";
  } else {
    modalTitle.textContent = "Import cognition.yaml";
    modalText.value = "";
  }
  modal.showModal();
}

document.querySelector("#export").onclick = () => openYAML("export");
document.querySelector("#import").onclick = () => openYAML("import");
document.querySelector("#vocab").onclick = () => openYAML("vocab");
document.querySelector("#reset").onclick = () => {
  store.replace(cloneConfig());
  show(toolFromHash());
};
document.querySelector("#modal-close").onclick = () => modal.close();
document.querySelector("#modal-copy").onclick = async () => {
  try {
    await navigator.clipboard.writeText(modalText.value);
    modalNote.textContent = "Copied.";
  } catch {
    modalNote.textContent = "Clipboard blocked. Select the text and copy it yourself.";
  }
};
document.querySelector("#modal-apply").onclick = () => {
  try {
    if (modalMode === "export") {
      modal.close();
      return;
    }
    const next = modalMode === "vocab"
      ? parseVocabulary(modalText.value, store.config)
      : parseYAML(modalText.value, store.config);
    store.replace(next);
    modal.close();
    show(toolFromHash());
  } catch (error) {
    modalNote.textContent = error.message;
  }
};

bootSim().then(() => {
  window.addEventListener("hashchange", () => show(toolFromHash()));
  show(toolFromHash());
}).catch(error => {
  host.textContent = error.message;
});
