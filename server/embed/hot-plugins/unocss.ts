/** @version: 0.58.0 */

import {
  createGenerator,
  type UnoGenerator,
  type UserConfig,
} from "https://esm.sh/@unocss/core@0.58.0";
import presetWind from "https://esm.sh/@unocss/preset-wind@0.58.0?bundle";

export default {
  name: "unocss",
  setup(hot: any) {
    const unoConfig: UserConfig = {
      presets: [presetWind()],
    };
    let uno: UnoGenerator;
    hot.unocss = {
      config: (config: UserConfig) => {
        Object.assign(unoConfig, config);
      },
    };
    hot.onLoad(
      /(^|\/|\.)uno.css$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        const lines = source.split("\n");
        const entryPoints = lines.filter((line) =>
          line.startsWith("@include ")
        );
        const entry = entryPoints.map((line) =>
          line.slice(9).split(",").map((s) => s.trim()).filter(Boolean)
        ).flat();
        const data = await Promise.all(entry.map((entryPoint) => {
          return fetch(entryPoint).then((res) => res.text());
        }));
        const { css } = await (uno ?? (uno = createGenerator(unoConfig)))
          .generate(data.join("\n"), {
            preflights: true,
            minify: true,
          });
        return { code: css, contentType: "text/css; charset=utf-8" };
      },
      "eager",
    );
  },
};
