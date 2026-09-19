// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/SAP/rootstock/pkg/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.SetDefaultLogger(log.WithLevel(log.InfoLvl.String()))

	logger = log.Default().New(log.WithName("test"))

	m.Run()
}

func TestParameterKey(t *testing.T) {
	t.Parallel()

	origRootKey := cfgRootKey

	t.Cleanup(func() { cfgRootKey = origRootKey })

	tests := []struct {
		name        string
		cfgRootKey  string
		relativeKey string
		baseKey     string
		wantKey     string
	}{
		{
			name:        "relativeKey only when no root key or base key",
			cfgRootKey:  "",
			relativeKey: "log.level",
			wantKey:     "log.level",
		},
		{
			name:        "cfgRootKey prepended when no base key",
			cfgRootKey:  "rootstock",
			relativeKey: "log.level",
			wantKey:     "rootstock.log.level",
		},
		{
			name:        "baseKey overrides cfgRootKey",
			cfgRootKey:  "rootstock",
			relativeKey: "password",
			baseKey:     "secrets",
			wantKey:     "secrets.password",
		},
		{
			name:        "baseKey used when cfgRootKey is empty",
			cfgRootKey:  "",
			relativeKey: "username",
			baseKey:     "secrets",
			wantKey:     "secrets.username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgRootKey = tt.cfgRootKey

			p := NewParameter(tt.relativeKey, "ENVVAR", "cli-flag", "usage", "default")
			if tt.baseKey != "" {
				p = p.WithBaseKey(tt.baseKey)
			}

			assert.Equal(t, tt.wantKey, p.Key())
		})
	}
}

func TestParameterRegister(t *testing.T) {
	t.Parallel()

	origRootKey := cfgRootKey

	t.Cleanup(func() {
		cfgRootKey = origRootKey

		viper.Reset()
	})

	cfgRootKey = ""

	tests := []struct {
		name         string
		registerFn   func(fs *pflag.FlagSet)
		flagName     string
		shorthand    string
		wantDefValue string
		wantUsage    string
	}{
		{
			name: "string parameter",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("str.key", "STR_ENV", "str-flag", "string usage", "strval").Register(fs)
			},
			flagName:     "str-flag",
			wantDefValue: "strval",
			wantUsage:    "string usage",
		},
		{
			name: "int parameter",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("int.key", "INT_ENV", "int-flag", "int usage", 99).Register(fs)
			},
			flagName:     "int-flag",
			wantDefValue: "99",
			wantUsage:    "int usage",
		},
		{
			name: "int parameter with shorthand",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("int.key", "INT_ENV", "int-flag", "int usage", 99).
					WithShorthand("z").Register(fs)
			},
			flagName:     "int-flag",
			wantDefValue: "99",
			shorthand:    "z",
			wantUsage:    "int usage",
		},
		{
			name: "bool parameter",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("bool.key", "BOOL_ENV", "bool-flag", "bool usage", true).Register(fs)
			},
			flagName:     "bool-flag",
			wantDefValue: "true",
			wantUsage:    "bool usage",
		},
		{
			name: "bool parameter with shorthand",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("bool.key", "BOOL_ENV", "bool-flag", "bool usage", true).
					WithShorthand("z").Register(fs)
			},
			flagName:     "bool-flag",
			wantDefValue: "true",
			shorthand:    "z",
			wantUsage:    "bool usage",
		},
		{
			name: "string parameter with shorthand",
			registerFn: func(fs *pflag.FlagSet) {
				NewParameter("short.key", "SHORT_ENV", "short-flag", "short usage", "sv").
					WithShorthand("z").Register(fs)
			},
			flagName:     "short-flag",
			shorthand:    "z",
			wantDefValue: "sv",
			wantUsage:    "short usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			viper.Reset()

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			tt.registerFn(fs)

			f := fs.Lookup(tt.flagName)
			require.NotNil(t, f, "expected flag %q to be registered", tt.flagName)
			assert.Equal(t, tt.wantDefValue, f.DefValue)
			assert.Contains(t, f.Usage, tt.wantUsage)

			if tt.shorthand != "" {
				sf := fs.ShorthandLookup(tt.shorthand)
				require.NotNil(t, sf, "expected shorthand %q to be registered", tt.shorthand)
			}
		})
	}
}

//nolint:revive // too many literals as test values in this function
func TestParameterValue(t *testing.T) {
	relativeKey := "rel.key"
	envVar := "ENV_VAR"
	cliFlag := "cli-flag"
	usage := "usage"

	tests := []struct {
		name                        string
		fromEnv, fromCLI            string
		fromSet, defaultValue, want any
	}{
		{
			name:         "string: returns default when registered but not set",
			defaultValue: "default-str",
			want:         "default-str",
		},
		{
			name:         "string: returns env var value",
			defaultValue: "default-str",
			fromEnv:      "from-env",
			want:         "from-env",
		},
		{
			name:         "string: returns parsed flag value",
			defaultValue: "default-str",
			fromEnv:      "from-env",
			fromCLI:      "from-cli",
			want:         "from-cli",
		},
		{
			name:         "string: returns viper.Set value",
			defaultValue: "default-str",
			fromEnv:      "from-env",
			fromCLI:      "from-cli",
			fromSet:      "from-set",
			want:         "from-set",
		},

		{
			name:         "int: returns default when registered but not set",
			defaultValue: 3,
			want:         3,
		},
		{
			name:         "int: returns env var value",
			defaultValue: 3,
			fromEnv:      "4",
			want:         4,
		},
		{
			name:         "int: returns parsed flag value",
			defaultValue: 3,
			fromEnv:      "4",
			fromCLI:      "5",
			want:         5,
		},
		{
			name:         "int: returns viper.Set value",
			defaultValue: 3,
			fromEnv:      "4",
			fromCLI:      "5",
			fromSet:      6,
			want:         6,
		},

		{
			name:         "bool: returns default when registered but not set",
			defaultValue: true,
			want:         true,
		},
		{
			name:         "bool: returns env var value",
			defaultValue: false,
			fromEnv:      "true",
			want:         true,
		},
		{
			name:         "bool: returns parsed flag value",
			defaultValue: false,
			fromEnv:      "false",
			fromCLI:      "true",
			want:         true,
		},
		{
			name:         "bool: returns viper.Set value",
			defaultValue: false,
			fromEnv:      "false",
			fromCLI:      "false",
			fromSet:      true,
			want:         true,
		},
	}

	origRootKey := cfgRootKey

	t.Cleanup(func() {
		cfgRootKey = origRootKey

		viper.Reset()
	})

	cfgRootKey = ""

	i := 0
	for _, tt := range tests {
		i++

		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			fs := pflag.NewFlagSet(fmt.Sprintf("test-%d", i), pflag.ContinueOnError)

			switch v := tt.defaultValue.(type) {
			case string:
				p := NewParameter(relativeKey, envVar, cliFlag, usage, v)
				p.Register(fs)
				prepareInputs(t, fs, cliFlag, tt.fromCLI, envVar, tt.fromEnv, p.Key(), tt.fromSet)
				assert.EqualValues(t, tt.want, p.Value())
			case bool:
				p := NewParameter(relativeKey, envVar, cliFlag, usage, v)
				p.Register(fs)
				prepareInputs(t, fs, cliFlag, tt.fromCLI, envVar, tt.fromEnv, p.Key(), tt.fromSet)
				assert.EqualValues(t, tt.want, p.Value())
			case int:
				p := NewParameter(relativeKey, envVar, cliFlag, usage, v)
				p.Register(fs)
				prepareInputs(t, fs, cliFlag, tt.fromCLI, envVar, tt.fromEnv, p.Key(), tt.fromSet)
				assert.EqualValues(t, tt.want, p.Value())
			default:
				t.Error("illegal parameter type")
				t.FailNow()
			}
		})
	}
}

func prepareInputs(t *testing.T, fs *pflag.FlagSet,
	cliFlag, cliVal string,
	envName, envVal string,
	viperKey string, viperVal any,
) {
	t.Helper()

	if cliVal != "" {
		require.NoError(t, fs.Parse([]string{"--" + cliFlag, cliVal}))
	}

	if envVal != "" {
		t.Setenv(envName, envVal)
	}

	if viperVal != nil {
		viper.Set(viperKey, viperVal)
	}
}

func TestCreateDirIfNotExist(t *testing.T) {
	t.Parallel()

	origMkdir := mkdir
	origIsDir := isDir

	t.Cleanup(func() {
		mkdir = origMkdir
		isDir = origIsDir
	})

	tests := []struct {
		name        string
		isDirResult func(string) (bool, error)
		expectMkdir bool
		mkdirErr    error
		wantErr     string
	}{
		{
			name: "creates directory when it does not exist",
			isDirResult: func(_ string) (bool, error) {
				return false, os.ErrNotExist
			},
			expectMkdir: true,
		},
		{
			name: "returns error when mkdir fails on missing dir",
			isDirResult: func(_ string) (bool, error) {
				return false, os.ErrNotExist
			},
			expectMkdir: true,
			mkdirErr:    errors.New("permission denied"),
			wantErr:     "unable to create directory",
		},
		{
			name: "succeeds when directory already exists",
			isDirResult: func(_ string) (bool, error) {
				return true, nil
			},
			expectMkdir: false,
		},
		{
			name: "returns error when path exists but is not a directory",
			isDirResult: func(_ string) (bool, error) {
				return false, nil
			},
			expectMkdir: false,
			wantErr:     "is not a folder",
		},
		{
			name: "returns error on unexpected stat error",
			isDirResult: func(_ string) (bool, error) {
				return false, errors.New("unexpected stat error")
			},
			expectMkdir: false,
			wantErr:     "no stat for directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mkdirCalled := false
			isDir = tt.isDirResult
			mkdir = func(_ string, _ os.FileMode) error {
				mkdirCalled = true
				return tt.mkdirErr
			}

			err := CreateDirIfNotExist("/some/test/dir")

			assert.Equal(t, tt.expectMkdir, mkdirCalled, "mkdir call expectation mismatch")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
