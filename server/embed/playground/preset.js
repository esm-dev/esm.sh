export default {
  'index.html': `<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width" />
  <link rel="stylesheet" href="./style.css" />
</head>
<body>
  <div id="root"></div>
  <script type="module" src="./app.jsx"></script>
</body>
</html>`,
  'style.css': `h1 {
  font-family: Arial, Helvetica, sans-serif;
}`,
  'app.jsx': `import React from '${location.origin}/react'
import ReactDom from '${location.origin}/react-dom'
  
ReactDom.render(
  <p>Hello World!</p>,
  document.getElementById('root')
)`
}