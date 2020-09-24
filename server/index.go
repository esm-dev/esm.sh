package server

const indexHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charSet="utf-8" />
    <meta name="viewport" content="user-scalable=no,initial-scale=1.0,minimum-scale=1.0,maximum-scale=1.0,minimal-ui" />
    <title>ESM</title>
</head>
<body>
    <main><em>Loading...</em></main>
    <script type="module">
        import React from '/react@16.13.1'
        import ReactDom from '/react-dom@16.13.1'

        ReactDom.render(
            React.createElement('div', null,
                React.createElement('h1', null, 'ESM'),
                React.createElement('p', null, 
                    React.createElement('a', { href: "https://github.com/postui/esmm" }, "github")
                ),
            ),
            document.querySelector('main')
        )
    </script>
</body>
</html>
`
