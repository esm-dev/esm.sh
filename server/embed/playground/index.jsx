import localforage from '/localforage'
import ReactDom from '/react-dom@17'
import React, { useEffect, useRef, useState } from '/react@17'
import { createEditor, createModel } from './editor.js'
import preset from './preset.js'

localforage.getItem('file-index.html').then(value => {
  if (!value) {
    localforage.setItem('current-file', 'index.html')
    Object.entries(preset).forEach(([name, content]) => {
      localforage.setItem(`file-${name}`, content)
    })
  }
})

function App() {
  const [siderWidth, setSiderWidth] = useState(100)
  const [editorWidth, setEditorWidth] = useState(0.5)
  const [files, setFiles] = useState(null)
  const [currentFile, setCurrentFile] = useState(null)
  const editorRef = useRef()
  const editorContainerRef = useRef()

  useEffect(() => {
    (async () => {
      let files = []
      let currentFile = null
      const indexHtml = await localforage.getItem('file-index.html')
      if (indexHtml) {
        const keys = await localforage.keys()
        files = await Promise.all(keys.map(async key => {
          if (key.startsWith('file-')) {
            const name = key.slice(5)
            const source = await localforage.getItem(key)
            const model = createModel(name, source)
            return { name, model }
          }
        }))
        currentFile = await localforage.getItem('current-file')
      } else {
        Object.entries(preset).forEach(([name, content]) => {
          files.push({ name, model: createModel(name, content) })
          localforage.setItem(`file-${name}`, content)
        })
        currentFile = 'index.html'
        localforage.setItem('current-file', currentFile)
      }
      editorRef.current = createEditor(editorContainerRef.current)
      setFiles(files.filter(Boolean))
      setCurrentFile(currentFile)
    })()
  }, [])

  useEffect(() => {
    if (files && currentFile) {
      const file = files.find(file => file.name == currentFile)
      if (file && editorRef.current) {
        editorRef.current.setModel(file.model)
      }
    }
  }, [currentFile, files])

  return (
    <>
      <div className="sider" style={{ width: siderWidth }} >
        {!files && <div className="file-item loading"><em>loading...</em></div>}
        {files && files.map(file => {
          return (
            <div
              className={["file-item", currentFile === file.name && 'active'].filter(Boolean).join(' ')}
              onClick={() => setCurrentFile(file.name)}
              key={file.name}
            >
              <span>{file.name}</span>
            </div>
          )
        })}
        <div className="file-item add">
          <svg style={{ width: '1em', height: '1em' }} viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M14 8H8V14H6V8H0V6H6V0H8V6H14V8Z" fill="currentColor" />
          </svg>
        </div>
      </div>
      <div className="editor" style={{ left: siderWidth, width: `${editorWidth * 100}vw` }} ref={editorContainerRef} />
      <div className="preview" style={{ right: 0, width: `calc(${(1 - editorWidth) * 100}vw - ${siderWidth}px)` }} >
        <iframe src={"/embed/playground/index.html"}></iframe>
      </div>
    </>
  )
}

ReactDom.render(<App />, document.getElementById('root'))
