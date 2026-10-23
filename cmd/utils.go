// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/cloudoperators/rootstock/internal/shutdown"
)

const (
	fmtEnvVars = "%s (env: %q)"
)

// NewParameter creates a cli parameter instance.
//
//   - relativeKey: dot-separated configKey of the parameter in config file. Per default this value
//     is assumed to be relative and the clis default root-level key is added.
//     If the WithBaseKey method is called, this key is relative to the specified base-key.
//
//     E.g. root-level key: "operator" and relative RelativeName: "log.level"
//     then the path in config-file is: ".operator.log.level"
//
//   - envVar: environment variable name of the parameter (per convention capitalized)
//
//   - cliFlag: cli parameter name (without dashes)
//
//   - usage: description of the parameter (showing up in tool tip)
//
//   - defaultValue: default value of the parameter if not set in config, envVar or via cliFlag
func NewParameter[T int | bool | string](
	relativeKey, envVar, cliFlag, usage string, defaultValue T,
) CLIParameter[T] {
	return &parameter[T]{
		relativeKey:  relativeKey,
		envVar:       envVar,
		cliFlag:      cliFlag,
		usage:        usage,
		defaultValue: defaultValue,
	}
}

type CLIParameter[T int | bool | string] interface {
	// returns the full dot-separated configKey of the parameter (including base key)
	Key() string
	// sets a base key in the config file for this parameter
	WithBaseKey(base string) CLIParameter[T]
	// sets a 1 character cli shorthand (without dash)
	WithShorthand(shorthand string) CLIParameter[T]
	// overwrites the usage field of the cli parameter
	WithUsage(usage string) CLIParameter[T]
	// Overwrites the default value in the cli parameter
	WithDefault(defaultValue T) CLIParameter[T]
	// returns the value associated with the cli parameter
	// (first match from cli-parameter, environment-var, config file, default)
	Value() T
	// Registers the defined cli parameter with the given cobra flagset
	Register(fs *pflag.FlagSet)
}

// parameter is the configuration input for registering a new comandline parameter.
type parameter[T int | bool | string] struct {
	// dot-separated configKey of the parameter in config file. Per default this value
	// is assumed to be relative and the clis default root-level key is added.
	// If the WithBaseKey method is called, this key is relative to the specified base-key.
	//
	// E.g. root-level key: "operator" and relative RelativeName: "log.level"
	// then the path in config-file is: ".operator.log.level"
	relativeKey string
	// environment variable name of the parameter (per convention capitalized)
	envVar string
	// cli parameter name (without dashes)
	cliFlag string
	// // 1 letter shorthand (without dashes) for the cli parameter. Will only be set
	// // if non-empty
	// CliShorthand string
	// description of the parameter (showing up in tool tip)
	usage string
	// default value of the parameter if not set elsewhere
	defaultValue T

	baseKey      string
	cliShorthand string
}

func (p parameter[T]) Key() string {
	if p.baseKey != "" {
		return p.baseKey + "." + p.relativeKey
	}

	if cfgRootKey != "" {
		return cfgRootKey + "." + p.relativeKey
	}

	return p.relativeKey
}

func (p *parameter[T]) WithBaseKey(base string) CLIParameter[T] {
	p.baseKey = base
	return p
}

func (p *parameter[T]) WithShorthand(s string) CLIParameter[T] {
	p.cliShorthand = s
	return p
}

func (p *parameter[T]) WithUsage(s string) CLIParameter[T] {
	p.usage = s
	return p
}

func (p *parameter[T]) WithDefault(d T) CLIParameter[T] {
	p.defaultValue = d
	return p
}

//nolint:revive,forcetypeassert // the type casting will always work due to the type constraints of T
func (p parameter[T]) Value() T {
	switch any(p.defaultValue).(type) {
	case bool:
		return any(viper.GetBool(p.Key())).(T)
	case string:
		return any(viper.GetString(p.Key())).(T)
	case int:
		return any(viper.GetInt(p.Key())).(T)
	default:
		cobra.CheckErr(fmt.Errorf("unsupported type %T for flag %s", p.defaultValue, p.cliFlag))

		var zero T

		return zero
	}
}

func (p parameter[T]) Register(fs *pflag.FlagSet) {
	if p.cliShorthand == "" {
		switch defaultVal := any(p.defaultValue).(type) {
		case string:
			fs.StringVar(new(string), p.cliFlag, defaultVal, fmt.Sprintf(fmtEnvVars, p.usage, p.envVar))
		case int:
			fs.IntVar(new(int), p.cliFlag, defaultVal, fmt.Sprintf(fmtEnvVars, p.usage, p.envVar))
		case bool:
			fs.BoolVar(new(bool), p.cliFlag, defaultVal, fmt.Sprintf(fmtEnvVars, p.usage, p.envVar))
		}
	} else {
		switch defaultVal := any(p.defaultValue).(type) {
		case string:
			fs.StringVarP(
				new(string),
				p.cliFlag, p.cliShorthand, defaultVal,
				fmt.Sprintf(fmtEnvVars, p.usage, p.envVar),
			)
		case int:
			fs.IntVarP(
				new(int),
				p.cliFlag, p.cliShorthand, defaultVal,
				fmt.Sprintf(fmtEnvVars, p.usage, p.envVar),
			)
		case bool:
			fs.BoolVarP(
				new(bool),
				p.cliFlag, p.cliShorthand, defaultVal,
				fmt.Sprintf(fmtEnvVars, p.usage, p.envVar),
			)
		}
	}

	cobra.CheckErr(viper.BindEnv(p.Key(), p.envVar))
	cobra.CheckErr(viper.BindPFlag(p.Key(), fs.Lookup(p.cliFlag)))
}

func fatalOnError(err error, msg string) {
	if err == nil {
		return
	}

	logger.Error(msg, "err", err)
	shutdown.StopNow()
}

var (
	// Used for monkey patching in tests.
	mkdir = os.Mkdir
	isDir = checkDir
)

func createDirIfNotExist(dir string) error {
	is, err := isDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err = mkdir(dir, os.ModePerm); err != nil {
			return fmt.Errorf("unable to create directory %q: %w", dir, err)
		}
	case err == nil:
		if !is {
			return fmt.Errorf("unable to create directory %q: is not a folder", dir)
		}
	default:
		return fmt.Errorf("no stat for directory %q: %w", dir, err)
	}

	return nil
}

func checkDir(dir string) (bool, error) {
	stat, err := os.Stat(dir)
	if err != nil {
		return false, err
	}

	return stat.IsDir(), nil
}
