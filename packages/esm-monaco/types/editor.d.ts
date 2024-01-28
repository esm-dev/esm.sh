import { editor } from "./manaco.api";

export function init(
  root: HTMLElement,
  options: editor.IStandaloneEditorConstructionOptions & {
    languages?: string[];
  },
): Promise<editor.IStandaloneCodeEditor>;

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
