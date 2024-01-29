/**
 * Identifiers with a binary dot operator.
 * Examples: `baz` or `foo.bar`
 */
type ScopeName = string;

/**
 * An expression language of ScopePathStr with a binary comma (to indicate alternatives) operator.
 * Examples: `foo.bar boo.baz,quick quack`
 */
type ScopePattern = string;

declare const ruleIdSymbol: unique symbol;
type RuleId = {
  __brand: typeof ruleIdSymbol;
};

interface ILocation {
  readonly filename: string;
  readonly line: number;
  readonly char: number;
}
interface ILocatable {
  readonly $vscodeTextmateLocation?: ILocation;
}
type IncludeString = string;
type RegExpString = string;
interface IRawRepositoryMap {
  [name: string]: IRawRule;
  $self: IRawRule;
  $base: IRawRule;
}
type IRawRepository = IRawRepositoryMap & ILocatable;
interface IRawRule extends ILocatable {
  id?: RuleId;
  readonly include?: IncludeString;
  readonly name?: ScopeName;
  readonly contentName?: ScopeName;
  readonly match?: RegExpString;
  readonly captures?: IRawCaptures;
  readonly begin?: RegExpString;
  readonly beginCaptures?: IRawCaptures;
  readonly end?: RegExpString;
  readonly endCaptures?: IRawCaptures;
  readonly while?: RegExpString;
  readonly whileCaptures?: IRawCaptures;
  readonly patterns?: IRawRule[];
  readonly repository?: IRawRepository;
  readonly applyEndPatternLast?: boolean;
}
type IRawCaptures = IRawCapturesMap & ILocatable;
interface IRawCapturesMap {
  [captureId: string]: IRawRule;
}
interface IRawGrammar extends ILocatable {
  repository: IRawRepository;
  readonly scopeName: ScopeName;
  readonly patterns: IRawRule[];
  readonly injections?: {
    [expression: string]: IRawRule;
  };
  readonly injectionSelector?: string;
  readonly fileTypes?: string[];
  readonly name?: string;
  readonly firstLineMatch?: string;
}

export interface LanguageRegistration extends IRawGrammar {
  name: string;
  scopeName: string;
  displayName?: string;
  aliases?: string[];
  /**
   * A list of languages the current language embeds.
   * If manually specifying languages to load, make sure to load the embedded
   * languages for each parent language.
   */
  embeddedLangs?: string[];
  /**
   * A list of languages that embed the current language.
   * Unlike `embeddedLangs`, the embedded languages will not be loaded automatically.
   */
  embeddedLangsLazy?: string[];
  balancedBracketSelectors?: string[];
  unbalancedBracketSelectors?: string[];
  foldingStopMarker?: string;
  foldingStartMarker?: string;
  /**
   * Inject this language to other scopes.
   * Same as `injectTo` in VSCode's `contributes.grammars`.
   *
   * @see https://code.visualstudio.com/api/language-extensions/syntax-highlight-guide#injection-grammars
   */
  injectTo?: string[];
}

/**
 * A TextMate theme.
 */
interface IRawTheme {
  readonly name?: string;
  readonly settings: IRawThemeSetting[];
}

/**
 * A single theme setting.
 */
interface IRawThemeSetting {
  readonly name?: string;
  readonly scope?: ScopePattern | ScopePattern[];
  readonly settings: {
    readonly fontStyle?: string;
    readonly foreground?: string;
    readonly background?: string;
  };
}

interface ThemeRegistrationRaw
  extends IRawTheme, Partial<Omit<ThemeRegistration, "name" | "settings">> {
}

interface ThemeRegistration extends Partial<ThemeRegistrationResolved> {
}

interface ThemeRegistrationResolved extends IRawTheme {
  /**
   * Theme name
   */
  name: string;
  /**
   * Display name
   *
   * @field shiki custom property
   */
  displayName?: string;
  /**
   * Light/dark theme
   *
   * @field shiki custom property
   */
  type: "light" | "dark";
  /**
   * Token rules
   */
  settings: IRawThemeSetting[];
  /**
   * Same as `settings`, will use as fallback if `settings` is not present.
   */
  tokenColors?: IRawThemeSetting[];
  /**
   * Default foreground color
   *
   * @field shiki custom property
   */
  fg: string;
  /**
   * Background color
   *
   * @field shiki custom property
   */
  bg: string;
  /**
   * A map of color names to new color values.
   *
   * The color key starts with '#' and should be lowercased.
   *
   * @field shiki custom property
   */
  colorReplacements?: Record<string, string>;
  /**
   * Color map of VS Code options
   *
   * Will be used by shiki on `lang: 'ansi'` to find ANSI colors, and to find the default foreground/background colors.
   */
  colors?: Record<string, string>;
  /**
   * JSON schema path
   *
   * @field not used by shiki
   */
  $schema?: string;
  /**
   * Enable semantic highlighting
   *
   * @field not used by shiki
   */
  semanticHighlighting?: boolean;
  /**
   * Tokens for semantic highlighting
   *
   * @field not used by shiki
   */
  semanticTokenColors?: Record<string, string>;
}

export type ThemeRegistrationAny =
  | ThemeRegistrationRaw
  | ThemeRegistration
  | ThemeRegistrationResolved;

export type BundledTheme =
  | "andromeeda"
  | "aurora-x"
  | "ayu-dark"
  | "catppuccin-frappe"
  | "catppuccin-latte"
  | "catppuccin-macchiato"
  | "catppuccin-mocha"
  | "dark-plus"
  | "dracula"
  | "dracula-soft"
  | "github-dark"
  | "github-dark-dimmed"
  | "github-light"
  | "light-plus"
  | "material-theme"
  | "material-theme-darker"
  | "material-theme-lighter"
  | "material-theme-ocean"
  | "material-theme-palenight"
  | "min-dark"
  | "min-light"
  | "monokai"
  | "night-owl"
  | "nord"
  | "one-dark-pro"
  | "poimandres"
  | "red"
  | "rose-pine"
  | "rose-pine-dawn"
  | "rose-pine-moon"
  | "slack-dark"
  | "slack-ochin"
  | "solarized-dark"
  | "solarized-light"
  | "synthwave-84"
  | "tokyo-night"
  | "vitesse-black"
  | "vitesse-dark"
  | "vitesse-light";

export type BundledLanguage =
  | "abap"
  | "actionscript-3"
  | "ada"
  | "angular-html"
  | "angular-ts"
  | "apache"
  | "apex"
  | "apl"
  | "applescript"
  | "ara"
  | "asm"
  | "astro"
  | "awk"
  | "ballerina"
  | "bash"
  | "bat"
  | "batch"
  | "be"
  | "beancount"
  | "berry"
  | "bibtex"
  | "bicep"
  | "blade"
  | "c"
  | "c#"
  | "c++"
  | "cadence"
  | "cdc"
  | "clarity"
  | "clj"
  | "clojure"
  | "cmake"
  | "cmd"
  | "cobol"
  | "codeql"
  | "coffee"
  | "coffeescript"
  | "console"
  | "cpp"
  | "cql"
  | "crystal"
  | "cs"
  | "csharp"
  | "css"
  | "csv"
  | "cue"
  | "cypher"
  | "d"
  | "dart"
  | "dax"
  | "diff"
  | "docker"
  | "dockerfile"
  | "dream-maker"
  | "elixir"
  | "elm"
  | "erb"
  | "erl"
  | "erlang"
  | "f#"
  | "fish"
  | "fs"
  | "fsharp"
  | "fsl"
  | "gdresource"
  | "gdscript"
  | "gdshader"
  | "gherkin"
  | "git-commit"
  | "git-rebase"
  | "gjs"
  | "glimmer-js"
  | "glimmer-ts"
  | "glsl"
  | "gnuplot"
  | "go"
  | "gql"
  | "graphql"
  | "groovy"
  | "gts"
  | "hack"
  | "haml"
  | "handlebars"
  | "haskell"
  | "hbs"
  | "hcl"
  | "hjson"
  | "hlsl"
  | "hs"
  | "html"
  | "http"
  | "imba"
  | "ini"
  | "jade"
  | "java"
  | "javascript"
  | "jinja"
  | "jison"
  | "js"
  | "json"
  | "json5"
  | "jsonc"
  | "jsonl"
  | "jsonnet"
  | "jssm"
  | "jsx"
  | "julia"
  | "kotlin"
  | "kql"
  | "kt"
  | "kts"
  | "kusto"
  | "latex"
  | "less"
  | "liquid"
  | "lisp"
  | "logo"
  | "lua"
  | "make"
  | "makefile"
  | "markdown"
  | "marko"
  | "matlab"
  | "md"
  | "mdc"
  | "mdx"
  | "mermaid"
  | "mojo"
  | "nar"
  | "narrat"
  | "nextflow"
  | "nf"
  | "nginx"
  | "nim"
  | "nix"
  | "nu"
  | "nushell"
  | "objc"
  | "objective-c"
  | "objective-cpp"
  | "ocaml"
  | "pascal"
  | "perl"
  | "perl6"
  | "php"
  | "plsql"
  | "postcss"
  | "powerquery"
  | "powershell"
  | "prisma"
  | "prolog"
  | "properties"
  | "proto"
  | "ps"
  | "ps1"
  | "pug"
  | "puppet"
  | "purescript"
  | "py"
  | "python"
  | "ql"
  | "r"
  | "raku"
  | "razor"
  | "rb"
  | "reg"
  | "rel"
  | "riscv"
  | "rs"
  | "rst"
  | "ruby"
  | "rust"
  | "sas"
  | "sass"
  | "scala"
  | "scheme"
  | "scss"
  | "sh"
  | "shader"
  | "shaderlab"
  | "shell"
  | "shellscript"
  | "shellsession"
  | "smalltalk"
  | "solidity"
  | "sparql"
  | "spl"
  | "splunk"
  | "sql"
  | "ssh-config"
  | "stata"
  | "styl"
  | "stylus"
  | "svelte"
  | "swift"
  | "system-verilog"
  | "tasl"
  | "tcl"
  | "tex"
  | "toml"
  | "ts"
  | "tsx"
  | "turtle"
  | "twig"
  | "typescript"
  | "v"
  | "vb"
  | "verilog"
  | "vhdl"
  | "vim"
  | "viml"
  | "vimscript"
  | "vue"
  | "vue-html"
  | "vy"
  | "vyper"
  | "wasm"
  | "wenyan"
  | "wgsl"
  | "wl"
  | "wolfram"
  | "xml"
  | "xsl"
  | "yaml"
  | "yml"
  | "zenscript"
  | "zig"
  | "zsh"
  | "文言";
