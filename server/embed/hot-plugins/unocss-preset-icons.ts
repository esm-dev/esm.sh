/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import presetIcons from "https://esm.sh/@unocss/preset-icons@0.58.0?bundle";

export default {
  name: "unocss-preset-icons",
  setup(hot: Hot) {
    const unocssPresets = hot.unocssPresets ?? (hot.unocssPresets = []);
    unocssPresets.push((config) =>
      presetIcons({
        cdn: "https://esm.sh/",
        ...config.presetIcons,
      })
    );
  },
};
