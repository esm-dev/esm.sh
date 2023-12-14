import type { AttributifyOptions } from "https://esm.sh/@unocss/preset-attributify@0.58.0?bundle";

declare global {
  interface UnoConfig {
    presetAttributify?: AttributifyOptions;
  }
}
