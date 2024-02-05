import type { ThemeInput } from "@shikijs/core";
import type { LanguageInput, LanguageRegistration } from "@shikijs/core";
import loadWasm from "@shikijs/core/wasm-inlined";
import { getHighlighterCore } from "@shikijs/core";
import { version as tmGrammersVersion } from "../node_modules/tm-grammars/package.json";
import { version as tmThemesVersion } from "../node_modules/tm-themes/package.json";
import { vfetch } from "./vfs";

// @ts-expect-error `TM_GRAMMARS` is defined at build time
const tmGrammars: { name: string; aliases?: string[] }[] = TM_GRAMMARS;
// @ts-expect-error `TM_THEMES` is defined at build time
const tmThemes: Set<string> = new Set(TM_THEMES);

export const tmGrammerRegistry = new Set(tmGrammars.map((l) => l.name));
export const loadedGrammars = new Set<string>();

export interface ShikiInitOptions {
  theme?: string | { name: string };
  preloadGrammars?: string[];
  customGrammars?: { name: string }[];
}

export async function initShiki({
  theme = "vitesse-dark",
  preloadGrammars = [],
  customGrammars,
}: ShikiInitOptions) {
  const langs: LanguageInput = [];
  const themes: ThemeInput[] = [];

  if (preloadGrammars) {
    langs.push(
      ...await Promise.all(
        tmGrammars.filter((g) =>
          preloadGrammars.includes(g.name) ||
          g.aliases?.some((a) => preloadGrammars.includes(a))
        ).map((g) => {
          loadedGrammars.add(g.name);
          return loadTMGrammer(g.name);
        }),
      ),
    );
  }

  if (customGrammars) {
    for (const lang of customGrammars) {
      if (
        typeof lang === "object" && lang !== null && lang.name &&
        !tmGrammerRegistry.has(lang.name)
      ) {
        tmGrammerRegistry.add(lang.name);
        loadedGrammars.add(lang.name);
        langs.push(lang as LanguageRegistration);
      }
    }
  }

  if (typeof theme === "string") {
    if (tmThemes.has(theme)) {
      themes.push(loadTMTheme(theme));
    }
  } else if (typeof theme === "object" && theme !== null && theme.name) {
    themes.push(theme);
  }

  return getHighlighterCore({ themes, langs, loadWasm });
}

export function getLanguageIdFromPath(path: string) {
  const idx = path.lastIndexOf(".");
  if (idx > 0) {
    const ext = path.slice(idx + 1);
    const lang = tmGrammars.find((g) =>
      g.name === ext || g.aliases?.includes(ext)
    );
    if (lang) {
      return lang.name;
    }
  }
}

export function loadTMTheme(theme: string) {
  return vfetch(
    `https://esm.sh/tm-themes@${tmThemesVersion}/themes/${theme}.json`,
  ).then((res) => res.json());
}

export function loadTMGrammer(id: string) {
  return vfetch(
    `https://esm.sh/tm-grammars@${tmGrammersVersion}/grammars/${id}.json`,
  ).then((res) => res.json());
}

export { tmGrammars, tmThemes };
