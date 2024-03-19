import * as marked from "{origin}{basePath}/marked@12.0.1";
import hljs from "{origin}{basePath}/highlight.js@11.9.0/lib/core";
import javascript from "{origin}{basePath}/highlight.js@11.9.0/lib/languages/javascript";
import json from "{origin}{basePath}/highlight.js@11.9.0/lib/languages/json";
import xml from "{origin}{basePath}/highlight.js@11.9.0/lib/languages/xml";
import bash from "{origin}{basePath}/highlight.js@11.9.0/lib/languages/bash";

export function render(md) {
  const mainEl = document.querySelector("main");
  mainEl.innerHTML = marked.parse(md.split("# esm.sh")[1]);
  mainEl.querySelectorAll("h1,h2,h3").forEach((el) => {
    el.id = el.textContent.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  });
  mainEl.querySelectorAll("code.language-bash").forEach((block) => {
    block.innerHTML = block.innerHTML.replace(/(^|\n)\$ /g, "$1");
  });

  // scroll to hashHeading
  const hashHeading = document.getElementById(location.hash.slice(1));
  if (hashHeading) {
    hashHeading.scrollIntoView();
  }

  hljs.registerLanguage("javascript", javascript);
  hljs.registerLanguage("json", json);
  hljs.registerLanguage("jsonc", json);
  hljs.registerLanguage("xml", xml);
  hljs.registerLanguage("bash", (hljs) => {
    const l = bash(hljs);
    l.keywords.built_in = "cd git sh";
    return l;
  });
  hljs.highlightAll();
}
