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
  'app.jsx': `import React from 'https://esm.sh/react'
import ReactDom from 'https://esm.sh/react-dom'
  
ReactDom.render(<p>Hello World!</p>, document.getElementById('root'))`
}