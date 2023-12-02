import fs from "node:fs";
import * as vscode from "vscode";
import { Attribute, SaxEventType, SAXParser, Tag, Text } from "sax-wasm";

const saxPath = require.resolve("sax-wasm/lib/sax-wasm.wasm");
const saxWasmBuffer = fs.readFileSync(saxPath);

export async function getImportMap(document: vscode.TextDocument) {
  const options = { highWaterMark: 32 * 1024 }; // 32k chunks
  const parser = new SAXParser(
    SaxEventType.Text | SaxEventType.OpenTag | SaxEventType.CloseTag,
    options,
  );
  await parser.prepareWasm(saxWasmBuffer);

  return new Promise<any>((resolve, reject) => {
    let inScript = false;
    let scriptType = "";
    let done = false;
    parser.eventHandler = (event, data) => {
      switch (event) {
        case SaxEventType.OpenTag: {
          const tag = data as Tag;
          if (tag.name === "script") {
            inScript = true;
            tag.attributes.forEach((attr) => {
              if (attr.name.value === "type") {
                scriptType = attr.value.value;
              }
            });
          }
          break;
        }
        case SaxEventType.CloseTag: {
          if ((data as Tag).name === "script") {
            inScript = false;
          }
          break;
        }
        case SaxEventType.Text: {
          if (inScript && scriptType === "importmap") {
            try {
              resolve(JSON.parse((data as Text).value));
            } catch (e) {
              console.warn("Failed to parse importmap", e);
              reject(null);
            }
            done = true;
          }
          break;
        }
      }
    };
    parser.write(new TextEncoder().encode(document.getText()));
    parser.end();
    if (!done) {
      resolve(null);
    }
  });
}
