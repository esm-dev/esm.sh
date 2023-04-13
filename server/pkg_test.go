package server

import (
	"testing"
)

func TestPkgPath(t *testing.T) {
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
