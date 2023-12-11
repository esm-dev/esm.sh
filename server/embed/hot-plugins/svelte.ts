/** @version: 4.2.7 */

import type { Hot } from "../types/hot.d.ts";
import { compile } from "https://esm.sh/svelte@4.2.7/compiler";

export default {
  name: "svelte",
  setup(hot: Hot) {
    hot.onLoad(
      /\.svelte$/,
      (url, source, options) => {
        const { importMap, isDev } = options;
        const { js } = compile(source, {
          filename: url.pathname,
          sveltePath: importMap.imports?.["svelte/"] && importMap.$support
            ? "svelte"
            : (importMap.imports?.["svelte"] ?? "https://esm.sh/svelte@4.2.7"),
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
    );
  },
};
