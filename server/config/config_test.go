package config

import (
	"testing"
)

func TestExtractPackageName(t *testing.T) {
	type want struct {
		fullNameWithoutVersion  string
		scope                   string
		nameWithoutVersionScope string
	}
	tests := []struct {
		name        string
		packageName string
		want        want
	}{
		{
			name:        "PackageWithVersionAndNoScope",
			packageName: "faker@1.5.0",
			want:        want{fullNameWithoutVersion: "faker", scope: "", nameWithoutVersionScope: "faker"},
		},
		{
			name:        "PackageWithVersionAndScope",
			packageName: "@github/faker@1.5.0",
			want:        want{fullNameWithoutVersion: "@github/faker", scope: "@github", nameWithoutVersionScope: "faker"},
		},
		{
			name:        "ReactLoadedFromStable",
			packageName: "react@18.2.0/es2022/react.mjs",
			want:        want{fullNameWithoutVersion: "react", scope: "", nameWithoutVersionScope: "react"},
		},
		{
			name:        "ScopedLoadedFromStable",
			packageName: "@github/faker@0.0.1/es2022/faker.mjs",
			want:        want{fullNameWithoutVersion: "@github/faker", scope: "@github", nameWithoutVersionScope: "faker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullNameWithoutVersion, scope, nameWithoutVersionScope := extractPackageName(tt.packageName)

			if fullNameWithoutVersion != tt.want.fullNameWithoutVersion {
				t.Errorf("%s not equal %s", fullNameWithoutVersion, tt.want.fullNameWithoutVersion)
			}
			if scope != tt.want.scope {
				t.Errorf("%s not equal %s", scope, tt.want.scope)
			}
			if nameWithoutVersionScope != tt.want.nameWithoutVersionScope {
				t.Errorf("%s not equal %s", nameWithoutVersionScope, tt.want.nameWithoutVersionScope)
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
				Scopes: []AllowScope{{
					Name: "@github",
				}},
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
				Scopes: []AllowScope{{
					Name: "@github",
				}},
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
				Scopes: []AllowScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@github/perfect"},
			want: true,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []AllowScope{{
					Name: "@github",
				}},
			},
			args: args{fullName: "@faker/perfect"},
			want: false,
		},
		{
			name: "NotAllowedByScope",
			allowList: AllowList{
				Scopes: []AllowScope{{
					Name: "@github",
				}},
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
