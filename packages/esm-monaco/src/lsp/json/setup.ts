import * as monacoNS from "monaco-editor-core";

export function setup(id: string, monaco: typeof monacoNS) {
  console.log("setup", id, monaco);
}
