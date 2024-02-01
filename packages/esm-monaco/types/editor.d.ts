import type { BundledLanguage, BundledTheme } from "./shiki";
import type { GrammarInfo } from "./tm-grammars";
import type { ThemeInfo } from "./tm-themes";
import type { VFS } from "./vfs";

export interface InitOptions {
  vfs?: VFS;
  themes?: (BundledTheme | ThemeInfo)[];
  preloadGrammers?: BundledLanguage[];
  customGrammers?: GrammarInfo[];
}

export function init(options?: InitOptions): Promise<void>;

export * from "./monaco";
export * from "./vfs";
