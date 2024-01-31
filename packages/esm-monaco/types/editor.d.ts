import type { editor, IDisposable, Uri } from "./monaco";
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

export function create(
  container: HTMLElement,
  options: editor.IStandaloneEditorConstructionOptions,
): editor.IStandaloneCodeEditor;

export function createModel(
  value: string,
  language?: string,
  uri?: string | Uri,
): editor.ITextModel;

interface TypescriptAPI {
  addExtraLib(content: string, filePath?: string): IDisposable;
}

export namespace languages {
  export const javascript: TypescriptAPI;
  export const typescript: TypescriptAPI;
}

export * from "./monaco";
export * from "./vfs";
