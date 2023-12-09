/** @version: 4.2.7 */

import { compile } from "https://esm.sh/svelte@4.2.7/compiler";

export default {
  name: "svelte",
  setup(hot: any) {
    hot.onLoad(
      /\.svelte$/,
      (url: URL, source: string, options: Record<string, any> = {}) => {
        const { isDev } = hot;
        const { importMap } = options;
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
