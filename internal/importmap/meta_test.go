package importmap

import "testing"

func TestParseEsmPath(t *testing.T) {
	for _, test := range []struct {
		path string
		want Import
	}{
		{"/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "pkg", Version: "1.0.0"}},
		{"/*pkg@1.0.0/es2022/sub.mjs", Import{Name: "pkg", Version: "1.0.0", SubPath: "sub"}},
		{"/@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0"}},
		{"/*@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0"}},
		{"/*@scope/pkg@1.0.0/es2022/sub.development.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", SubPath: "sub", Dev: true}},
		{"/*@scope/pkg/sub/file.js", Import{Name: "@scope/pkg", SubPath: "sub/file.js"}},
		{"/gh/owner/repo@1.0.0", Import{Name: "owner/repo", Version: "1.0.0", Github: true}},
		{"/gh/owner/repo@1.0.0/es2022/repo.mjs", Import{Name: "owner/repo", Version: "1.0.0", Github: true}},
		{"/gh/owner/repo@main/src/index.js", Import{Name: "owner/repo", Version: "main", SubPath: "src/index.js", Github: true}},
		{"/gh/owner/repo/es2022/sub/file.mjs", Import{Name: "owner/repo", SubPath: "sub/file", Github: true}},
		{"/gh/*owner/repo@1.0.0/es2022/repo.development.mjs", Import{Name: "owner/repo", Version: "1.0.0", Github: true, Dev: true}},
		{"/*gh/owner/repo@1.0.0/es2022/sub.mjs", Import{Name: "owner/repo", Version: "1.0.0", SubPath: "sub", Github: true}},
		{"/jsr/@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", Jsr: true}},
		{"/jsr/*@scope/pkg@1.0.0/es2022/sub.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", SubPath: "sub", Jsr: true}},
		{"/*jsr/@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", Jsr: true}},
	} {
		for _, prefix := range []string{"", "https://esm.sh", "http://localhost:8080"} {
			path := prefix + test.path + "?target=es2022#module"
			t.Run(path, func(t *testing.T) {
				got, err := ParseEsmPath(path)
				if err != nil || got != test.want {
					t.Fatalf("ParseEsmPath(%q) = %+v, %v; want %+v", path, got, err, test.want)
				}
			})
		}
	}
}

func TestParseEsmPathInvalid(t *testing.T) {
	for _, path := range []string{"/", "/*", "/@scope", "/*@scope", "/*@scope/", "/gh/", "/gh/owner", "/gh/owner/", "/gh/*owner", "/gh/*owner/", "/*gh/owner/"} {
		t.Run(path, func(t *testing.T) {
			if got, err := ParseEsmPath(path); err == nil {
				t.Fatalf("ParseEsmPath(%q) = %+v; want an error", path, got)
			}
		})
	}
}
