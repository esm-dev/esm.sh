/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import MagicString from "https://esm.sh/magic-string@0.30.5?bundle";
import { transformDirectives } from "https://esm.sh/@unocss/transformer-directives@0.58.0?bundle";

export default {
  name: "unocss-transform-directives",
  setup(hot: Hot) {
    hot.unocssTransformDirectives = async (code, uno) => {
      const ms = new MagicString(code);
      await transformDirectives(ms, uno, { throwOnMissing: false });
      return ms.toString();
    };
  },
};
