# Web App Handler for Go

> [!WARNING]
> The `web` package is still in development. Use it at your own risk.

A golang `http.Handler` that serves _nobuild_ web applications.

- Web applications are served _as-is_ without any build step.
- Transpiles TypeScript, JSX, Vue, Svelte _on-the-fly_.
- Built-in [TailwindCSS](https://tailwindcss.com) and [UnoCSS](https://unocss.dev) generator.
- Static files are served from the application directory.
- Support Hot Module Replacement (HMR) for development

## Installation

```sh
go get -u github.com/esm-dev/esm.sh
```

## Usage

Create a web server that serves the web application from a directory:

```go
package main

import (
  "net/http"
  "log"

  "github.com/esm-dev/esm.sh/web"
)

func main() {
  http.Handle("GET /", web.NewHandler(web.Config{
    AppDir: "/path/to/webapp",
    Fallback: "/index.html", // fallback to root index.html (SPA mode)
    Dev: false, // change to `true` to enable HMR
  }))
  log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Create a `index.html` file in the web application directory:

```html
<!DOCTYPE html>
<html>
<head>
  <title>My Web Application</title>
  <link rel="stylesheet" href="/tailwind.css">
  <script type="importmap">
    {
      "imports": {
        "react": "https://esm.sh/react@19.2.3/es2022/react.mjs",
        "react/": "https://esm.sh/react@19.2.3/",
        "react-dom": "https://esm.sh/*react-dom@19.2.3/es2022/react-dom.mjs",
        "react-dom/": "https://esm.sh/*react-dom@19.2.3/"
      }
    }
  </script>
</head>
<body class="flex justify-center items-center h-screen">
  <div id="app"></div>
  <script type="module" src="/app.tsx"></script>
</body>
</html>
```

Create a `app.tsx` file in the web application directory:

```tsx
import { createRoot } from "react-dom/client"

function App() {
  return <h1>Hello, World!</h1>
}

createRoot(document.getElementById("app")).render(<App />)
```

Create a `tailwind.css` file in the web application directory:

```css
@import "tailwindcss";
```

Run the web server:

```sh
go run .
```

Open the web browser and navigate to `http://localhost:8080`.
