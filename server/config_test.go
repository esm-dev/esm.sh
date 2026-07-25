package server

import (
	"testing"
)

func TestCdnOrigin(t *testing.T) {
	t.Setenv("CDN_ORIGIN", "")
	for origin, want := range map[string]string{
		"https://cdn.example.com/": "https://cdn.example.com",
		"http://localhost:8080":    "http://localhost:8080",
		"https://[::1]:8443":       "https://[::1]:8443",
	} {
		config := &Config{CdnOrigin: origin}
		if err := normalizeConfig(config); err != nil {
			t.Fatal(err)
		}
		if config.CdnOrigin != want {
			t.Fatalf("unexpected CDN origin %q, want %q", config.CdnOrigin, want)
		}
	}

	config := &Config{Origin: "https://legacy.example.com"}
	if err := normalizeConfig(config); err != nil {
		t.Fatal(err)
	}
	if config.CdnOrigin != config.Origin {
		t.Fatalf("unexpected legacy CDN origin %q", config.CdnOrigin)
	}

	for _, origin := range []string{
		"//cdn.example.com",
		"https://:443",
		"https://user@cdn.example.com",
		`https://cdn.example";globalThis.PWNED=1;"`,
		"https://cdn.example';globalThis.PWNED=1;'",
		`https://cdn.example\evil`,
		"https://bad_host.example",
		"https://[cdn.example]",
		"https://-cdn.example",
		"https://cdn-.example",
		"https://cdn.example:",
		"https://cdn.example:0",
		"https://cdn.example:65536",
		"https://cdn.example.com/path",
		"https://cdn.example.com?",
		"https://cdn.example.com?query",
		"https://cdn.example.com#fragment",
		"javascript:alert(1)",
		"https://cdn.example.com\r\nX-Pwned: 1",
	} {
		if err := normalizeConfig(&Config{CdnOrigin: origin}); err == nil {
			t.Errorf("expected CDN origin %q to be rejected", origin)
		}
	}
}

func TestExtractPackageName(t *testing.T) {
	type want struct {
		packageId string
		scope     string
		name      string
		version   string
	}
	tests := []struct {
		name        string
		packageName string
		want        want
	}{
		{
			name:        "PackageWithVersionAndNoScope",
			packageName: "faker@1.5.0",
			want:        want{packageId: "faker@1.5.0", scope: "", name: "faker", version: "1.5.0"},
		},
		{
			name:        "PackageWithVersionAndScope",
			packageName: "@github/faker@1.5.0",
			want:        want{packageId: "@github/faker@1.5.0", scope: "@github", name: "faker", version: "1.5.0"},
		},
		{
			name:        "ReactLoadedFromStable",
			packageName: "react@18.2.0/es2022/react.mjs",
			want:        want{packageId: "react@18.2.0", scope: "", name: "react", version: "18.2.0"},
		},
		{
			name:        "ScopedLoadedFromStable",
			packageName: "@github/faker@0.0.1/es2022/faker.mjs",
			want:        want{packageId: "@github/faker@0.0.1", scope: "@github", name: "faker", version: "0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullNameWithoutVersion, scope, name, version := extractPackageName(tt.packageName)

			if fullNameWithoutVersion != tt.want.packageId {
				t.Errorf("%s not equal %s", fullNameWithoutVersion, tt.want.packageId)
			}
			if scope != tt.want.scope {
				t.Errorf("%s not equal %s", scope, tt.want.scope)
			}
			if name != tt.want.name {
				t.Errorf("%s not equal %s", name, tt.want.name)
			}
			if version != tt.want.version {
				t.Errorf("%s not equal %s", version, tt.want.version)
			}
		})
	}
}

func TestAllowListAndBanList_IsPackageNotAllowedOrBanned(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name      string
		allowList AllowList
		banList   BanList
		args      args
		want      bool
	}{
		{
			name:      "NoAllowOrBanListAllowAnything",
			allowList: AllowList{},
			banList:   BanList{},
			args:      args{fullName: "faker@1.5.0"},
			want:      false,
		},
		{
			name: "AllowedScopeBannedScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			banList: BanList{
				Scopes: []BanScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
		{
			name: "AllowedScopeBannedPackage",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			banList: BanList{
				Packages: []string{"@github/faker"},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
		{
			name: "AllowedPackageBannedPackage",
			allowList: AllowList{
				Packages: []string{"@github/faker"},
			},
			banList: BanList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "AllowedPackageBannedScope",
			allowList: AllowList{
				Packages: []string{"faker"},
			},
			banList: BanList{
				Scopes: []BanScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@github/faker"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// to simulate:
			// if !pkgAllowed || pkgBanned {
			//   return rex.Status(403, "forbidden")
			// }
			packageName := tt.args.fullName

			isAllowed := tt.allowList.IsPackageAllowed(packageName)
			isBanned := tt.banList.IsPackageBanned(packageName)

			if got := !isAllowed || isBanned; got != tt.want {
				t.Errorf("isPackageNotAllowedOrBanned() = %v, want %v. %v isAllowed %v, %v isBanned %v", got, tt.want, packageName, isAllowed, packageName, isBanned)
			}
		})
	}
}

func TestAllowList_IsPackageAllowed(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name      string
		allowList AllowList
		args      args
		want      bool
	}{
		{
			name:      "NoAllowListAllowAnything",
			allowList: AllowList{},
			args:      args{fullName: "faker@1.5.0"},
			want:      true,
		},
		{
			name: "AllowedByPackages",
			allowList: AllowList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "NotAllowedByPackages",
			allowList: AllowList{
				Packages: []string{"allowedPackageName"},
			},
			args: args{fullName: "faker"},
			want: false,
		},
		{
			name: "AllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@github/perfect"},
			want: true,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@faker/perfect"},
			want: false,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []string{"@github"},
			},
			args: args{fullName: "@faker/perfect"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.allowList.IsPackageAllowed(tt.args.fullName); got != tt.want {
				t.Errorf("IsPackageAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBanList_IsPackageBanned(t *testing.T) {
	type args struct {
		fullName string
	}
	tests := []struct {
		name    string
		banList BanList
		args    args
		want    bool
	}{
		{
			name:    "NotBanned",
			banList: BanList{},
			args:    args{fullName: "faker@1.5.0"},
			want:    false,
		},
		{
			name: "BannedByPackages",
			banList: BanList{
				Packages: []string{"faker"},
			},
			args: args{fullName: "faker"},
			want: true,
		},
		{
			name: "BannedByScopes",
			banList: BanList{
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"perfect"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: true,
		},
		{
			name: "BannedByScopesButExcluded",
			banList: BanList{
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"faker"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: false,
		},
		{
			name: "ExcludedInScopeButBannedByPackages",
			banList: BanList{
				Packages: []string{"@github/faker"},
				Scopes: []BanScope{{
					Name:     "@github",
					Excludes: []string{"faker"},
				}},
			},
			args: args{fullName: "@github/faker@1.0.0"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.banList.IsPackageBanned(tt.args.fullName); got != tt.want {
				t.Errorf("IsPackageBanned() = %v, want %v", got, tt.want)
			}
		})
	}
}
