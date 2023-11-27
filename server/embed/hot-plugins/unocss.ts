/** @version: 0.57.7 */

import {
  createGenerator,
  type UnoGenerator,
  type UserConfig,
} from "https://esm.sh/@unocss/core@0.57.7";
import presetWind from "https://esm.sh/@unocss/preset-wind@0.57.7?bundle";

export default {
  name: "unocss",
  setup(hot: any) {
    const unoConfig: UserConfig = {
      presets: [presetWind()],
    };
    let entry: string | string[] = [];
    let uno: UnoGenerator;
    hot.unocss = {
      set entry(e: string | string[]) {
        entry = e;
      },
      config: (config: typeof unoConfig) => {
        Object.assign(unoConfig, config);
      },
    };
    hot.register(
      "uno.css",
      async () => {
        if (typeof entry === "string") {
          return fetch(entry).then((res) => res.text());
        }
        const a = await Promise.all(entry.map((entryPoint) => {
          return fetch(entryPoint).then((res) => res.text());
        }));
        return a.join("\n");
      },
      async (input: string) => {
        const { css } = await (uno ?? (uno = createGenerator(unoConfig)))
          .generate(input, {
            preflights: true,
            minify: true,
          });
        return css;
      },
    );
  },
};
