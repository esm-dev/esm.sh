/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import {
  createGenerator,
  type UnoGenerator,
  type UserConfig,
} from "https://esm.sh/@unocss/core@0.58.0";
import presetWind from "https://esm.sh/@unocss/preset-wind@0.58.0?bundle";

export default {
  name: "unocss",
  setup(hot: Hot) {
    const unoConfig: UserConfig = {
      presets: [presetWind()],
    };
    hot.unocss = {
      config(config: UserConfig) {
        Object.assign(unoConfig, config);
      },
    };
    let uno: UnoGenerator;
    hot.onLoad(
      /(^|\/|\.)uno.css$/,
      async (url: URL, source: string, _options: Record<string, any> = {}) => {
        const lines = source.split("\n");
        const entryPoints = lines.filter((line) =>
          line.startsWith("@include ")
        );
        const deps = entryPoints.map((line) =>
          line.slice(9).split(",").map((s) => s.trim()).filter(Boolean)
        ).flat().map((s) => new URL(s, url));
        const data = await Promise.all(deps.map((url) => {
          return fetch(url).then((res) => res.text());
        }));
        const { css } = await (uno ?? (uno = createGenerator(unoConfig)))
          .generate(data.join("\n"), {
            preflights: true,
            minify: true,
          });
        return {
          code: css,
          contentType: "text/css; charset=utf-8",
          deps: deps.map((url) => url.pathname),
        };
      },
      "eager",
    );
  },
};
