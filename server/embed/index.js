import * as marked from '/marked'
import hljs from '/highlight.js/lib/core'
import javascript from '/highlight.js/lib/languages/javascript'
import json from '/highlight.js/lib/languages/json'
import xml from '/highlight.js/lib/languages/xml'
import bash from '/highlight.js/lib/languages/bash'

export function render(md) {
  const mainEl = document.querySelector('main')
  mainEl.innerHTML = marked.parse(md).replaceAll('https://esm.sh', window.origin) + `<p class="link"><a href="/?playground">Playground &rarr; </a></p><p class="link"><a href="/?test">Testing &rarr; </a></p>`
  mainEl.removeChild(mainEl.querySelector('h1'))
  mainEl.querySelectorAll('code.language-bash').forEach(block => {
    block.innerHTML = block.innerHTML.replace(/(^|\n)\$ /g, '$1')
  })
  
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
