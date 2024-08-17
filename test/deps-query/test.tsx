import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("?deps", async () => {
  {
    const res = await fetch("http://localhost:8080/@mui/material@5.16.7?deps=react@18.2.0,react-dom@18.2.0,foo@0.0.0&target=es2022");
    const code = await res.text();
    assertStringIncludes(code, 'import "/react-dom@18.2.0/es2022/react-dom.mjs"');
    assertStringIncludes(code, 'import "/react@18.2.0/es2022/jsx-runtime.js"');
    assertStringIncludes(code, 'import "/react@18.2.0/es2022/react.mjs"');
    assertStringIncludes(code, 'import "/react-transition-group@^4.4.5?deps=react-dom@18.2.0,react@18.2.0&target=es2022"');
    assertStringIncludes(code, 'export * from "/@mui/material@5.16.7/X-ZHJlYWN0LWRvbUAxOC4yLjAscmVhY3RAMTguMi4w/es2022/material.mjs"');
    assertStringIncludes(code, 'import "/@mui/system@^5.16.7/createTheme?deps=react@18.2.0&target=es2022"');
    assertStringIncludes(code, 'import "/@mui/utils@^5.16.6/useTimeout?deps=react@18.2.0&target=es2022"');
  }
  {
    const res = await fetch("http://localhost:8080/@mui/material@5.16.7/X-ZHJlYWN0LWRvbUAxOC4yLjAscmVhY3RAMTguMi4w/es2022/material.mjs");
    const code = await res.text();
    assertStringIncludes(code, 'from"/react-dom@18.2.0/es2022/react-dom.mjs"');
    assertStringIncludes(code, 'from"/react@18.2.0/es2022/jsx-runtime.js"');
    assertStringIncludes(code, 'from"/react@18.2.0/es2022/react.mjs"');
    assertStringIncludes(code, 'from"/react-transition-group@^4.4.5?deps=react-dom@18.2.0,react@18.2.0&target=es2022"');
    assertStringIncludes(code, 'from"/@mui/system@^5.16.7/createTheme?deps=react@18.2.0&target=es2022"');
    assertStringIncludes(code, 'from"/@mui/utils@^5.16.6/useTimeout?deps=react@18.2.0&target=es2022"');
  }
});
