import type { SchemaConfiguration } from "vscode-json-languageservice";

export const schemas: SchemaConfiguration[] = [
  {
    uri:
      "https://raw.githubusercontent.com/denoland/vscode_deno/main/schemas/import_map.schema.json",
    fileMatch: [
      "import_map.json",
      "import-map.json",
      "importmap.json",
      "importMap.json",
    ],
  },
  {
    uri: "https://json.schemastore.org/tsconfig",
    fileMatch: ["tsconfig.json"],
  },
];
