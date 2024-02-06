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

export const grammarRegistry = new Set(tmGrammars.map((l) => l.name));
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

  if (preloadGrammars.length > 0) {
    langs.push(
      ...await Promise.all(
        preloadGrammars.map((src) =>
          src.startsWith("https://")
            ? { name: src }
            : tmGrammars.find((g) => g.name === src || g.aliases?.includes(src))
        ).filter(Boolean).map(({ name }) => {
          loadedGrammars.add(name);
          return loadTMGrammer(name);
        }),
      ),
    );
  }

  if (customGrammars) {
    for (const lang of customGrammars) {
      if (
        typeof lang === "object" && lang !== null && lang.name &&
        !grammarRegistry.has(lang.name)
      ) {
        grammarRegistry.add(lang.name);
        loadedGrammars.add(lang.name);
        langs.push(lang as LanguageRegistration);
      }
    }
  }

  if (typeof theme === "string") {
    if (tmThemes.has(theme) || theme.startsWith("https://")) {
      themes.push(loadTMTheme(theme));
    }
  } else if (typeof theme === "object" && theme !== null && theme.name) {
    themes.push(theme);
  }

  return getHighlighterCore({ langs, themes, loadWasm });
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

export function loadTMTheme(src: string) {
  const url = tmThemes.has(src)
    ? `https://esm.sh/tm-themes@${tmThemesVersion}/themes/${src}.json`
    : src;
  return vfetch(url).then((res) => res.json());
}

export function loadTMGrammer(src: string) {
  const grammar = tmGrammars.find((g) =>
    g.name === src || g.aliases?.includes(src)
  );
  const url = grammar
    ? `https://esm.sh/tm-grammars@${tmGrammersVersion}/grammars/${grammar.name}.json`
    : src;
  return vfetch(url).then((res) => res.json());
}

export { tmGrammars, tmThemes };
