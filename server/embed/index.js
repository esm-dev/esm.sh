import * as marked from '{origin}{basePath}/marked'
import hljs from '{origin}{basePath}/highlight.js/lib/core'
import javascript from '{origin}{basePath}/highlight.js/lib/languages/javascript'
import json from '{origin}{basePath}/highlight.js/lib/languages/json'
import xml from '{origin}{basePath}/highlight.js/lib/languages/xml'
import bash from '{origin}{basePath}/highlight.js/lib/languages/bash'

export function render(md) {
  const mainEl = document.querySelector('main')
  mainEl.innerHTML = marked.parse(md).replaceAll(/\{origin\}/g, '{origin}{basePath}') + `<p class="link"><a href="./?test">Testing &rarr; </a></p>`
  mainEl.removeChild(mainEl.querySelector('h1'))
  mainEl.querySelectorAll('code.language-bash').forEach(block => {
    block.innerHTML = block.innerHTML.replace(/(^|\n)\$ /g, '$1')
  })

  const fragment = document.getElementById(location.hash.slice(1))
  if (fragment) {
    fragment.scrollIntoView()
  }

  hljs.registerLanguage('javascript', javascript)
  hljs.registerLanguage('json', json)
  hljs.registerLanguage('xml', xml)
  hljs.registerLanguage('bash', hljs => {
    const l = bash(hljs)
    l.keywords.built_in = 'cd git sh'
    return l
  });
  hljs.highlightAll()
}
