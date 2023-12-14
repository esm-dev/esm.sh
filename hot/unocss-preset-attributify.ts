/** @version: 0.58.0 */

import type { Hot } from "../server/embed/types/hot.d.ts";
import presetAttributify from "https://esm.sh/@unocss/preset-attributify@0.58.0?bundle";

export default {
  name: "unocss-preset-attributify",
  setup(hot: Hot) {
    const unocssPresets = hot.unocssPresets ?? (hot.unocssPresets = []);
    unocssPresets.push((config) => presetAttributify(config.presetAttributify));
  },
};
