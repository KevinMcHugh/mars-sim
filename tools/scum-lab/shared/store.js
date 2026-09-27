// One cognition config for every Scum Lab tool. Tools mutate `store.config`
// in place (it is the object the grammar editors edit). Call `replace` when
// the whole document is swapped — import, reset — so the shell can remount.

import { DEFAULT_CONFIG } from "./defaults.js";

export function cloneConfig(config = DEFAULT_CONFIG) {
  return JSON.parse(JSON.stringify(config));
}

export const store = {
  config: cloneConfig(),
  replace(next) {
    this.config = next;
  },
};
