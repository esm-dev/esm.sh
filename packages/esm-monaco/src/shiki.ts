import type * as monacoNs from "monaco-editor-core";
import type {
  LanguageInput,
  LanguageRegistration,
  ThemeInput,
} from "@shikijs/core";
import { getHighlighterCore } from "@shikijs/core";
import { shikiToMonaco } from "@shikijs/monaco";
import { grammars as allGrammars } from "tm-grammars";
import { themes as allThemes } from "tm-themes";
import { version as tmGrammersVersion } from "../node_modules/tm-grammars/package.json";
import { version as tmThemesVersion } from "../node_modules/tm-themes/package.json";
import loadWasm from "@shikijs/core/wasm-inlined";

const allGrammerNames = new Set(allGrammars.map((l) => l.name));
const loadedGrammars = new Set<string>();

export async function initShiki(
  monaco: typeof monacoNs,
  options: {
    themes?: (string | { name: string })[];
    preloadGrammars?: string[];
    customGrammars?: { name: string }[];
    onLanguage?: (id: string) => void | Promise<void>;
  },
) {
  const themes: ThemeInput[] = [];
  const langs: LanguageInput = [];

  if (options.preloadGrammars) {
    const preloadGrammars = new Set(options.preloadGrammars);
    langs.push(
      ...await Promise.all(
        allGrammars.filter((g) =>
          preloadGrammars.has(g.name) ||
          g.aliases?.some((a) => preloadGrammars.has(a))
        ).map((g) => {
          loadedGrammars.add(g.name);
          return loadTMGrammer(g.name);
        }),
      ),
    );
  }

  if (options.customGrammars) {
    for (const lang of options.customGrammars) {
      if (
        typeof lang === "object" && lang !== null && lang.name &&
        !allGrammerNames.has(lang.name)
      ) {
        allGrammerNames.add(lang.name);
        loadedGrammars.add(lang.name);
        langs.push(lang as LanguageRegistration);
      }
    }
  }

  if (options.themes) {
    for (const theme of options.themes) {
      if (typeof theme === "string") {
        if (allThemes.some((t) => t.name === theme)) {
          themes.push(loadTMTheme(theme));
        }
      } else if (typeof theme === "object" && theme !== null && theme.name) {
        themes.push(theme);
      }
    }
  }

  const highlighter = await getHighlighterCore({ themes, langs, loadWasm });

  for (const id of allGrammerNames) {
    monaco.languages.register({ id });
    monaco.languages.onLanguage(id, async () => {
      if (!loadedGrammars.has(id)) {
        highlighter.loadLanguage(loadTMGrammer(id)).then(() => {
          // activate the highlighter for the language
          shikiToMonaco(highlighter, monaco);
        });
      }
      if (options.onLanguage) {
        await options.onLanguage(id);
      }
    });
  }

  shikiToMonaco(highlighter, monaco);
}

function loadTMTheme(theme: string) {
  return fetch(
    `https://esm.sh/tm-themes@${tmThemesVersion}/themes/${theme}.json`,
  ).then((res) => res.json());
}

function loadTMGrammer(lang: string) {
  return fetch(
    `https://esm.sh/tm-grammars@${tmGrammersVersion}/grammars/${lang}.json`,
  ).then((res) => res.json());
}

// add some aliases for javascript and typescript
const javascriptGrammar = allGrammars.find((g) => g.name === "javascript");
const typescriptGrammar = allGrammars.find((g) => g.name === "typescript");
javascriptGrammar.aliases?.push("mjs", "cjs", "jsx");
typescriptGrammar.aliases?.push("mts", "cts", "tsx");

export function getLanguageIdFromExtension(path: string) {
  const idx = path.lastIndexOf(".");
  if (idx > 0) {
    const ext = path.slice(idx + 1);
    const lang = allGrammars.find((g) =>
      g.name === ext || g.aliases?.includes(ext)
    );
    if (lang) {
      return lang.name;
    }
  }
}

export { allGrammars, allThemes };
