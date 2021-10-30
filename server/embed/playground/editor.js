import { Uri, editor } from '/monaco-editor'
import localforage from '/localforage'
import editorWorker from '/monaco-editor/esm/vs/editor/editor.worker?worker'
import jsonWorker from '/monaco-editor/esm/vs/language/json/json.worker?worker'
import cssWorker from '/monaco-editor/esm/vs/language/css/css.worker?worker'
import htmlWorker from '/monaco-editor/esm/vs/language/html/html.worker?worker'
import tsWorker from '/monaco-editor/esm/vs/language/typescript/ts.worker?worker'
import '/monaco-editor?css'

self.MonacoEnvironment = {
  getWorker(_, label) {
    if (label === 'json') {
      return new jsonWorker()
    }
    if (label === 'css' || label === 'scss' || label === 'less') {
      return new cssWorker()
    }
    if (label === 'html' || label === 'handlebars' || label === 'razor') {
      return new htmlWorker()
    }
    if (label === 'typescript' || label === 'javascript') {
      return new tsWorker()
    }
    return new editorWorker()
  }
}

export function createModel(name, source) {
  const uri = Uri.parse(`file:///src/${name}`)
  const model = editor.createModel(source, getLanguage(name), uri)
  model.onDidChangeContent(e=>{
    localforage.setItem(`file-${name}`,model.getValue())
    console.log(e)
  })
  return model
}

export function createEditor(container) {
  return editor.create(container, {
    automaticLayout: true,
    contextmenu: true,
    fontFamily: '"Dank Mono", "Source Code Pro", monospace',
    fontLigatures: true,
    fontSize: 14,
    lineHeight: 18,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    smoothScrolling: true,
    scrollbar: {
      useShadows: false,
      verticalScrollbarSize: 10,
      horizontalScrollbarSize: 10
    },
    overviewRulerLanes: 0,
  })
}

export function getLanguage(name) {
  switch (name.slice(name.lastIndexOf('.') + 1).toLowerCase()) {
    case 'ts':
    case 'tsx':
      return 'typescript'
    case 'js':
    case 'jsx':
      return 'javascript'
    case 'json':
      return 'json'
    case 'css':
      return 'css'
    case 'html':
      return 'html'
  }
  return ''
}
