import { VFS } from "/index.js";

export const vfs = new VFS({
  scope: "test",
  initial: {
    "log.d.ts": [
      "/** log a message. */",
      "declare function log(message:string): void;",
    ],
    "greeting.ts": [
      'export const message = "Hello world!" as const;',
    ],
    "index.html": [
      "<!DOCTYPE html>",
      "<html>",
      "<head>",
      '  <meta charset="utf-8">',
      "  <title>React App</title>",
      '  <link rel="stylesheet" href="./style.css">',
      '  \<script type="importmap" src="import_map.json"><\/script>',
      "</head>",
      "<body>",
      '  <div id="root"></div>',
      '  <script type="module" src="./main.tsx"><\/script>',
      "</body>",
      "</html>",
    ],
    "style.css": [
      "h1 {",
      "  font-style: italic;",
      "}",
    ],
    "App.tsx": [
      'import confetti from "https://esm.sh/canvas-confetti@1.6.0"',
      'import { useEffect } from "react"',
      'import { message } from "./greeting.ts"',
      "",
      "export default function App() {",
      "  useEffect(() => {",
      "    confetti()",
      "    log(message)",
      "  }, [])",
      "  return <h1>{message}</h1>;",
      "}",
    ],
    "main.jsx": [
      'import { createRoot } from "react-dom/client"',
      'import App from "./App.tsx"',
      "",
      'const root = createRoot(document.getElementById("root"))',
      "root.render(<App />)",
    ],
    "import_map.json": JSON.stringify(
      {
        imports: {
          "@jsxImportSource": "https://esm.sh/react@18.2.0",
          "react": "https://esm.sh/react@18.2.0",
          "react-dom/": "https://esm.sh/react-dom@18.2.0/",
        },
      },
      null,
      2,
    ),
    "tsconfig.json": JSON.stringify(
      {
        compilerOptions: {
          types: [
            "log.d.ts",
            "https://raw.githubusercontent.com/vitejs/vite/main/packages/vite/types/importMeta.d.ts",
          ],
        },
      },
      null,
      2,
    ),
  },
});
