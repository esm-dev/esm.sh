/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import presetWebFonts from "https://esm.sh/@unocss/preset-web-fonts@0.58.0?bundle";

export default {
  name: "unocss-preset-web-fonts",
  setup(hot: Hot) {
    const unocssPresets = hot.unocssPresets ?? (hot.unocssPresets = []);
    unocssPresets.push((config) => presetWebFonts(config.presetWebFonts));
  },
};
