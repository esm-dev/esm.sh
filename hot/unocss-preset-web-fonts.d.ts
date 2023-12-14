import type { WebFontsOptions } from "https://esm.sh/@unocss/preset-web-fonts@0.58.0?bundle";

declare global {
  interface UnoConfig {
    presetWebFonts?: WebFontsOptions;
  }
}
