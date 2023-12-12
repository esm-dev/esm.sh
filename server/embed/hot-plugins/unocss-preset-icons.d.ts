import type { IconsOptions } from "https://esm.sh/@unocss/preset-icons@0.58.0?bundle";

declare global {
  interface UnoConfig {
    presetIcons?: IconsOptions;
  }
}
