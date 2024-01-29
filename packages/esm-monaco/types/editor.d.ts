import type { editor } from "./manaco.api";
import type { BundledTheme, LanguageRegistration, ThemeRegistrationAny } from "./shiki";

export interface InitOptions {
  themes?: (BundledTheme | ThemeRegistrationAny)[];
  customLanguages?: LanguageRegistration[];
}

export function init(options?: InitOptions): Promise<void>;

export function create(
  container: HTMLElement,
  options: editor.IStandaloneEditorConstructionOptions,
): editor.IStandaloneCodeEditor;

export function createModel(
  value: string,
  language?: string,
  uri?: string | monaco.Uri,
): editor.ITextModel;

export * from "./manaco.api";
