package config

import "testing"

func TestBanList_IsPackageBaned(t *testing.T) {
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
			if got := tt.banList.IsPackageBaned(tt.args.fullName); got != tt.want {
				t.Errorf("IsPackageBaned() = %v, want %v", got, tt.want)
			}
		})
	}
}
