// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	selectedCase := ""
	tests := []struct {
		name         string
		semVer       string
		buildDate    string
		gitCommit    string
		gitTag       string
		gitTreeState string
		wantErr      string
		wantRead     string
		wantMajor    int
		wantMinor    int
		wantPatch    int
	}{
		{
			name:         "happy path with tag",
			semVer:       "1.2.3-suf",
			buildDate:    "1970-01-01T00:00:00Z",
			gitCommit:    "abc123",
			gitTag:       "v1.2.3",
			gitTreeState: "clean",
			wantMajor:    1,
			wantMinor:    2,
			wantPatch:    3,
			wantRead:     "v1.2.3",
		},
		{
			name:         "dirty tree with commit",
			semVer:       "1.2.3",
			gitCommit:    "123456789",
			gitTreeState: "dirty",
			wantRead:     "v1.2.3+1234567.dirty",
		},
		{
			name:         "dirty tree without commit",
			semVer:       "1.2.3",
			gitTreeState: "dirty",
			wantRead:     "v1.2.3+unknown",
		},
		{
			name:    "not a semantic version",
			semVer:  "1.2",
			wantErr: "illegal version provided: ",
		},
		{
			name:    "non-integer major component",
			semVer:  ".2.1",
			wantErr: "failed to cast \"major\" version in semantic version ",
		},
		{
			name:    "non-integer minor component",
			semVer:  "1..2",
			wantErr: "failed to cast \"minor\" version in semantic version ",
		},
		{
			name:    "non-integer patch component",
			semVer:  "1.2.",
			wantErr: "failed to cast \"patch\" version in semantic version ",
		},
	}

	for _, tt := range tests {
		if selectedCase != "" && tt.name != selectedCase {
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			err := Init(tt.semVer, tt.buildDate, tt.gitCommit, tt.gitTag, tt.gitTreeState)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRead, Read())

			if tt.wantMajor+tt.wantMinor+tt.wantPatch > 0 {
				gotMajor, gotMinor, gotPatch := SemanticVersion()
				assert.Equal(t, tt.wantMajor, gotMajor)
				assert.Equal(t, tt.wantMinor, gotMinor)
				assert.Equal(t, tt.wantPatch, gotPatch)
			}
		})
	}
}

func TestReadAll(t *testing.T) {
	major, minor, patch := 1, 2, 3
	suffix := "-suf"
	semVer := fmt.Sprintf("%d.%d.%d%v", major, minor, patch, suffix)
	buildDate := "1970-01-01T00:00:00Z"
	gitCommit := "abc123"
	gitTag := "v1.2.3"
	gitTreeState := "clean"

	err := Init(semVer, buildDate, gitCommit, gitTag, gitTreeState)
	require.NoError(t, err)

	want := fmt.Sprintf("{"+
		"version:%s major:%d minor:%d patch:%d suffix:%s buildDate:%s gitCommit:%s "+
		"gitTag:%s gitTreeState:%s goVersion:%s compiler:%s platform:%s}",
		semVer, major, minor, patch, suffix, buildDate, gitCommit,
		gitTag, gitTreeState,
		runtime.Version(), runtime.Compiler,
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	)
	assert.Equal(t, want, ReadAll())
}
