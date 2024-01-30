import type { editor, Uri } from "./manaco.api";
import type { BundledLanguage, BundledTheme } from "./shiki";
import type { GrammarInfo } from "./tm-grammars";
import type { ThemeInfo } from "./tm-themes";

export interface InitOptions {
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

export * from "./manaco.api";
