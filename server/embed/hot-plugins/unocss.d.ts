import type {
  Preset,
  UnoGenerator,
  UserConfig,
} from "https://esm.sh/@unocss/core@0.58.0";
import type { PresetWindOptions } from "https://esm.sh/@unocss/preset-wind@0.58.0?bundle";

declare global {
  interface UnoConfig extends UserConfig {
    presetWind?: PresetWindOptions;
  }
  interface HotAPI {
    unocss: {
      config(config: UnoConfig): void;
    };
    unocssPresets?: ((config: UnoConfig) => Preset)[];
    unocssTransformDirectives?: (
      code: string,
      uno: UnoGenerator,
    ) => Promise<string>;
  }
}
