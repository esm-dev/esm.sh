import type { TypographyOptions } from "https://esm.sh/@unocss/preset-typography@0.58.0?bundle";

declare global {
  interface UnoConfig {
    presetTypography?: TypographyOptions;
  }
}
