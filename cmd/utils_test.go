// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
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
	logger = log.Default.New(log.WithName("test"))
	os.Exit(m.Run())
}

func TestParameterKey(t *testing.T) {
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

func TestParameterValue(t *testing.T) {
	origRootKey := cfgRootKey
	t.Cleanup(func() {
		cfgRootKey = origRootKey
		viper.Reset()
	})
	cfgRootKey = ""

	t.Run("string: returns default when registered but not set", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("str.key", "STR_ENV", "str-flag", "usage", "default-str")
		p.Register(fs)
		assert.Equal(t, "default-str", p.Value())
	})

	t.Run("string: returns viper.Set value", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("str.key", "STR_ENV", "str-flag", "usage", "default-str")
		p.Register(fs)
		viper.Set(p.Key(), "from-viper")
		assert.Equal(t, "from-viper", p.Value())
	})

	t.Run("string: returns env var value", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("str.key", "STR_ENV", "str-flag", "usage", "default-str")
		p.Register(fs)
		t.Setenv("STR_ENV", "from-env")
		assert.Equal(t, "from-env", p.Value())
	})

	t.Run("string: returns parsed flag value", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("str.key", "STR_ENV", "str-flag", "usage", "default-str")
		p.Register(fs)
		require.NoError(t, fs.Parse([]string{"--str-flag", "from-flag"}))
		assert.Equal(t, "from-flag", p.Value())
	})

	t.Run("int: returns default when registered but not set", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("int.key", "INT_ENV", "int-flag", "usage", 42)
		p.Register(fs)
		assert.Equal(t, 42, p.Value())
	})

	t.Run("int: returns viper.Set value", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("int.key", "INT_ENV", "int-flag", "usage", 42)
		p.Register(fs)
		viper.Set(p.Key(), 99)
		assert.Equal(t, 99, p.Value())
	})

	t.Run("bool: returns default when registered but not set", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("bool.key", "BOOL_ENV", "bool-flag", "usage", true)
		p.Register(fs)
		assert.Equal(t, true, p.Value())
	})

	t.Run("bool: returns viper.Set value", func(t *testing.T) {
		viper.Reset()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		p := NewParameter("bool.key", "BOOL_ENV", "bool-flag", "usage", true)
		p.Register(fs)
		viper.Set(p.Key(), false)
		assert.Equal(t, false, p.Value())
	})
}

func TestCreateDirIfNotExist(t *testing.T) {
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
