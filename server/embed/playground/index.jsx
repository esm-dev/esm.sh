import localforage from '/localforage'
import ReactDom from '/react-dom@17'
import React, { useCallback, useEffect, useRef, useState } from '/react@17'
import { createEditor, createModel } from './editor.js'
import preset from './preset.js'

document.title = 'ESM>CDN Playground'

localforage.getItem('file-index.html').then(value => {
	if (!value) {
		localforage.setItem('current-file', 'app.jsx')
		Object.entries(preset).forEach(([name, content]) => {
			localforage.setItem(`file-${name}`, content)
		})
	}
})

function App() {
	const [siderWidth, setSiderWidth] = useState(150)
	const [previewWidth, setPreviewWidth] = useState(0.4)
	const [previewUrl, setPreviewUrl] = useState('/embed/playground/index.html')
	const [files, setFiles] = useState(null)
	const [currentFile, setCurrentFile] = useState(null)
	const editorRef = useRef()
	const editorContainerRef = useRef()

	const addFile = useCallback(() => {
		let name
		if (name = prompt('Add New File:')) {
			const model = createModel(name, '')
			if (model) {
				setFiles(files => [...files, { name, model }])
				setCurrentFile(name)
			}
		}
	}, [])

	const refresh = useCallback(() => {
		setPreviewUrl('/embed/playground/index.html?' + Date.now())
	}, [])

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
				await Promise.all(Object.entries(preset).map(async ([name, content]) => {
					files.push({ name, model: createModel(name, content) })
					await localforage.setItem(`file-${name}`, content)
				}))
				currentFile = 'app.jsx'
				await localforage.setItem('current-file', currentFile)
				refresh()
			}
			editorRef.current = createEditor(editorContainerRef.current)
			setFiles(files.filter(Boolean).sort((a, b) => a.name > b.name ? 1 : -1))
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
				<div className="logo">
					<svg width="21" height="19" viewBox="0 0 21 19" fill="none" xmlns="http://www.w3.org/2000/svg">
						<path d="M14 2H7.54131C4.48093 2 2 4.23858 2 7C2 9.76142 4.48093 12 7.54131 12H14" stroke="#333" stroke-width="2.5" stroke-linecap="round" />
						<path d="M7 7H13.4587C16.5191 7 19 9.23858 19 12C19 14.7614 16.5191 17 13.4587 17H7.35294" stroke="#333" stroke-width="2.5" stroke-linecap="round" />
					</svg>
					Playground
				</div>
				{!files && <div className="file-item loading"><em>loading...</em></div>}
				{files && files.map(file => {
					return (
						<div
							className={["file-item", currentFile === file.name && 'active'].filter(Boolean).join(' ')}
							onClick={() => {
								setCurrentFile(file.name)
								localforage.setItem('current-file', file.name)
							}}
							key={file.name}
						>
							<span className="file-name">{file.name}</span>
						</div>
					)
				})}
				<div className="file-item add" onClick={addFile}>
					<svg style={{ width: '1em', height: '1em' }} viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg">
						<path d="M14 8H8V14H6V8H0V6H6V0H8V6H14V8Z" fill="currentColor" />
					</svg>
				</div>
			</div>
			<div className="editor" style={{ left: siderWidth, width: `calc(${(1 - previewWidth) * 100}vw - ${siderWidth}px)` }} ref={editorContainerRef} />
			{files && (
				<div className="preview" style={{ right: 0, width: `${previewWidth * 100}vw` }} >
					<iframe src={previewUrl}></iframe>
					<div className="refresh" onClick={refresh}>
						<svg style={{ width: '1em', height: '1em' }} viewBox="0 0 36 37" fill="none" xmlns="http://www.w3.org/2000/svg">
							<path d="M30.6914 5.27344L35.9648 0V15.8203H20.1445L27.4219 8.54297C24.75 5.87108 21.586 4.53516 17.9297 4.53516C14.2031 4.53516 11.0215 5.8535 8.38477 8.49023C5.74803 11.127 4.42969 14.3086 4.42969 18.0352C4.42969 21.7617 5.74803 24.9433 8.38477 27.5801C11.0215 30.2168 14.2031 31.5352 17.9297 31.5352C20.8828 31.5352 23.5195 30.709 25.8398 29.0566C28.1602 27.4043 29.7773 25.2422 30.6914 22.5703H35.332C34.3477 26.5078 32.2383 29.7422 29.0039 32.2734C25.7695 34.8047 22.0781 36.0703 17.9297 36.0703C13.0078 36.0703 8.78908 34.3125 5.27344 30.7969C1.7578 27.2812 0 23.0274 0 18.0352C0 13.0429 1.7578 8.78908 5.27344 5.27344C8.78908 1.7578 13.0078 0 17.9297 0C22.9219 0 27.1758 1.7578 30.6914 5.27344Z" fill="currentColor" />
						</svg>
					</div>
				</div>
			)}
		</>
	)
}

ReactDom.render(<App />, document.getElementById('root'))
