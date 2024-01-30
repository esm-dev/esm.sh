export type GrammarCategory = 'web' | 'markup' | 'general' | 'scripting' | 'data' | 'dsl' | 'utility' | 'config'

export interface GrammarInfo {
  name: string
  displayName: string
  categories?: GrammarCategory[]
  scopeName: string
  source: string
  aliases?: string[]
  licenseUrl?: string
  license?: string
  sha: string
  lastUpdate: string
  embedded?: string[]
  embeddedIn?: string[]
  byteSize: number
}

const grammars: GrammarInfo[]
const injections: GrammarInfo[]

export {
  grammars,
  injections,
}
