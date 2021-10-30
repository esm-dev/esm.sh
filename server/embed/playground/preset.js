export default {
  'index.html': `<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width" />
</head>
<body>
  <div id="root"></div>
  <script type="module" src="./app.jsx"></script>
</body>
</html>`,
  'app.jsx': `import React from '${location.origin}/react'
import ReactDom from '${location.origin}/react-dom'
  
ReactDom.render(<p>Hello World!</p>, document.getElementById('root'))`
}