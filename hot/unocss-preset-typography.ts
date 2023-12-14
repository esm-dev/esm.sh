/** @version: 0.58.0 */

import type { Hot } from "../server/embed/types/hot.d.ts";
import presetTypography from "https://esm.sh/@unocss/preset-typography@0.58.0?bundle";

export default {
  name: "unocss-preset-typography",
  setup(hot: Hot) {
    const unocssPresets = hot.unocssPresets ?? (hot.unocssPresets = []);
    unocssPresets.push((config) => presetTypography(config.presetTypography));
  },
};
