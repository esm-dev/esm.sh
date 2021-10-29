import React, { useEffect, useRef, useState } from '/react@17'
import ReactDom from '/react-dom@17'
import { createEditor } from './editor.js'

function App() {
	const [editorWidth, setEditorWidth] = useState(0.5)
	const editorContainerRef = useRef()

	useEffect(() => {
		createEditor(editorContainerRef.current)
	}, [])

	return (
		<>
			<div className="editor" style={{ width: editorWidth * 100 + '%' }} ref={editorContainerRef} />
		</>
	)
}

ReactDom.render(<App />, document.getElementById('root'))
