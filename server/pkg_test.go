package server

import (
	"encoding/json"
	"testing"
)

func TestPackageJsonParse(t *testing.T) {
	var info NpmPackageInfo
	err := json.Unmarshal([]byte(`{
		"name": "foo",
		"version": "1.0.0",
		"main": "index.js",
		"module": "index.mjs",
		"sideEffects": false,
		"esm.sh": {
			  "bundle": false
		  }
		}`), &info)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "foo" {
		t.Fatal("invalid name")
	}
	if info.Version != "1.0.0" {
		t.Fatal("invalid version")
	}
	if info.Main != "index.js" {
		t.Fatal("invalid main")
	}
	if info.Module != "index.mjs" {
		t.Fatal("invalid module")
	}
	if info.SideEffectsFalse != true {
		t.Fatal("invalid sideEffects")
	}
	if info.Esmsh["bundle"] != false {
		t.Fatal("invalid esm.sh config")
	}
}

func TestPkgPath(t *testing.T) {
	pkgName, pkgVersion, subPath := splitPkgPath("react")
	if pkgName != "react" || pkgVersion != "" || subPath != "" {
		t.Fatal("invalid splitPkgPath('react')")
	}
	pkgName, pkgVersion, subPath = splitPkgPath("react@18.2.0")
	if pkgName != "react" || pkgVersion != "18.2.0" || subPath != "" {
		t.Fatal("invalid splitPkgPath('react@18.2.0')")
	}
	pkgName, pkgVersion, subPath = splitPkgPath("react-dom@18.2.0/server")
	if pkgName != "react-dom" || pkgVersion != "18.2.0" || subPath != "server" {
		t.Fatal("invalid splitPkgPath('react@18.2.0/server')")
	}
	pkg, q, err := validatePkgPath("react@18.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q != "" {
		t.Fatalf("invalid unquery('%s'), should be empty", q)
	}
	if pkg.String() != "react@18.2.0" {
		t.Fatalf("invalid pkg('%v'), should be 'react@18.2.0'", pkg)
	}

	pkg, q, err = validatePkgPath("react-dom@18.2.0/client")
	if err != nil {
		t.Fatal(err)
	}
	if q != "" {
		t.Fatalf("invalid unquery('%s'), should be empty", q)
	}
	if pkg.String() != "react-dom@18.2.0/client" {
		t.Fatalf("invalid pkg('%v'), should be 'react-dom@18.2.0/client'", pkg)
	}

	pkg, q, err = validatePkgPath("react-dom@18.2.0&dev/client.js")
	if err != nil {
		t.Fatal(err)
	}
	if q != "dev" {
		t.Fatalf("invalid unquery('%s'), should be 'dev'", q)
	}
	if pkg.String() != "react-dom@18.2.0/client" {
		t.Fatalf("invalid pkg('%v'), should be 'react-dom@18.2.0/client'", pkg)
	}
}
