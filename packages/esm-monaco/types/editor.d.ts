import { editor } from "./manaco.api";

export function init(
  root: HTMLElement,
  options: editor.IStandaloneEditorConstructionOptions & {
    languages?: string[];
  },
): Promise<editor.IStandaloneCodeEditor>;

export * from "./manaco.api";
