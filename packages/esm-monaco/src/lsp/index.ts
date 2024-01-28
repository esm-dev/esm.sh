export interface LspMeta {
  id: string;
  api?: boolean;
}

export default <Record<string, LspMeta>> {
  html: { id: "html" },
  css: { id: "css" },
  json: { id: "json" },
  javascript: { id: "typescript", api: true },
  typescript: { id: "typescript", api: true },
};
