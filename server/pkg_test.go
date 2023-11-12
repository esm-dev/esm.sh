package server

import (
	"testing"
)

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

	pkg, q, err = validatePkgPath("@types/react@18.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q != "" {
		t.Fatalf("invalid unquery('%s'), should be empty", q)
	}
	if pkg.String() != "@types/react@"+fixedPkgVersions["@types/react@18"] {
		t.Fatalf("invalid pkg('%v'), should be '@types/react@%s'", pkg, fixedPkgVersions["@types/react@18"])
	}
}
