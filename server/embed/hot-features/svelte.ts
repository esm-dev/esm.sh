/** @version: 4.2.7 */

import { compile } from "https://esm.sh/v135/svelte@4.2.7/compiler";

export default {
  extnames: ["svelte"],
  transform: (url: URL, source: string, options: Record<string, any> = {}) => {
    const { isDev, importMap: _importMap } = options;
    const { js } = compile(source, {
      filename: url.pathname,
      generate: "dom",
      enableSourcemap: !!isDev,
      dev: !!isDev,
      css: "injected",
    });
    if (js.map) {
      js.map = JSON.stringify(js.map);
    }
    return js;
  },
};
