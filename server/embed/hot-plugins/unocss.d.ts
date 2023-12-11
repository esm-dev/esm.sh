import { type UserConfig } from "https://esm.sh/@unocss/core@0.58.0";

declare global {
  interface HotAPI {
    unocss: { config(config: UserConfig): void };
  }
}
