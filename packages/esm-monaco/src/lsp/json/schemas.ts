import type { SchemaConfiguration } from "vscode-json-languageservice";

export const schemas: SchemaConfiguration[] = [
  {
    uri: "//",
    fileMatch: [
      "import_map.json",
      "import-map.json",
      "importmap.json",
      "importMap.json",
    ],
    schema: {
      $schema: "http://json-schema.org/draft-07/schema#",
      $id:
        "https://github.com/denoland/vscode_deno/blob/main/schemas/import_map.schema.json",
      title: "An Import Map",
      description:
        "An import map which is used to remap imports when modules are loaded.",
      type: "object",
      properties: {
        imports: {
          description: "A map of specifiers to their remapped specifiers.",
          type: "object",
          properties: {
            "@jsxImportSource": {
              description: "The key is the specifier for JSX runtime.",
              type: "string",
            },
          },
          additionalProperties: {
            description:
              "The key is the specifier or partial specifier to match, with a value that represents the target specifier.",
            type: "string",
          },
        },
        scopes: {
          description:
            "Define a scope which remaps a specifier in only a specified scope",
          type: "object",
          additionalProperties: {
            description: "A definition of a scoped remapping.",
            type: "object",
            additionalProperties: {
              description:
                "The key is the specifier or partial specifier to match within the referring scope, with a value that represents the target specifier.",
              type: "string",
            },
          },
        },
      },
    },
  },
  {
    uri: "https://json.schemastore.org/tsconfig",
    fileMatch: [
      "tsconfig.json",
    ],
  },
];
