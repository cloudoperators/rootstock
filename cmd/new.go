// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SAP/rootstock/internal/shutdown"
	"github.com/SAP/rootstock/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	v "github.com/SAP/rootstock/pkg/version"
)

const (
	cliName = "rootstock"
)

var (
	logger log.FullInterface
)

func New() *cobra.Command {
	cobra.OnInitialize(initCLI)

	rootCmd := root(cliName)
	rootCmd.AddCommand(version())
	rootCmd.AddCommand(run())

	return rootCmd
}

var (
	onConfigChange = make([]func(), 0)
	cfgRootKey     = ""

	configPath = NewParameter(
		"config.path", "CONFIG_PATH", "config",
		"absolute config path",
		"set at runtime",
	)
	configFolder = NewParameter(
		"config.folder", "CONFIG_FOLDER", "config-folder",
		"folder where the config files are stored",
		"/config",
	)
	logLevel = NewParameter(
		"log.level", "LOG_LEVEL", "log-lvl",
		fmt.Sprintf("application log level options: key or value of %v", log.AllLevels()),
		log.InfoLvl.String(),
	).WithShorthand("l")
	logFormat = NewParameter(
		"log.format", "LOG_FORMAT", "log-format",
		"if this flag is set, logs will be presented in text format - options: text, json",
		"json",
	).WithShorthand("f")
	addTrace = NewParameter(
		"log.trace", "ADD_TRACE", "add-trace",
		"if this flag is set, error logs will have a stacktrace appended.",
		false,
	)
	tmpFolder = NewParameter(
		"tmpFolder", "TMP_FOLDER", "tmp-folder",
		"set at runtime",
		"",
	)
)

func initCLI() {
	workDir, err := os.Getwd()
	fatalOnError(err, "failed to read workdir")

	setupViper(cliName)

	// If a config file is found, read it in.
	configErr := viper.ReadInConfig()

	onConfigChange = append(onConfigChange, setLogger)
	setLogger()

	if configErr != nil {
		logger.Error("failed to read cobra config file", log.KeyError, configErr)
	}

	if tmpFolder.Value() == "" {
		tmpPath := filepath.Join(workDir, "tmp")
		viper.Set(tmpFolder.Key(), tmpPath)
		logger.Debug(
			"tmp folder not set - using default",
			"key", tmpFolder.Key(),
			"value", tmpFolder.Value(),
		)
	}

	err = CreateDirIfNotExist(tmpFolder.Value())
	fatalOnError(err, "failed to create tmp dir")

	logger.Debug("cli configured",
		"version", v.ReadAll(),
		"config file", viper.ConfigFileUsed(),
		"settings", viper.AllSettings(),
	)
}

func root(cliName string) *cobra.Command {
	cfgRootKey = cliName

	c := &cobra.Command{
		Use: cliName,
		Short: fmt.Sprintf(`
The %s cli gathers energy consumption information from available datacenter
apis and prepares it for reporting in standardized metrics.
		`, cliName,
		),
		Args: cobra.ExactArgs(1),
	}

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	configFolder.Register(c.PersistentFlags())
	configPath.
		WithDefault(filepath.Join(configFolder.Value(), fmt.Sprintf(".%s.yaml", cliName))).
		Register(c.PersistentFlags())
	logLevel.Register(c.PersistentFlags())
	logFormat.Register(c.PersistentFlags())
	addTrace.Register(c.PersistentFlags())

	tmpFolder.
		WithUsage("tmp folder used by " + cliName).
		Register(c.PersistentFlags())

	return c
}

func version() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Returns the cli version",
		Long:  `The version command returns service version and additional platform information`,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", "TODO")
			shutdown.StopNowWith(0)
		},
	}
}

func setupViper(cliName string) {
	viper.SetConfigType("yaml")

	if configPath.Value() != "" {
		viper.SetConfigFile(configPath.Value())
	} else {
		viper.AddConfigPath(configFolder.Value())
		viper.SetConfigName("." + cliName + ".yaml")
	}

	viper.AutomaticEnv() // read in environment variables that match

	viper.OnConfigChange(onFileChange(updateServiceConfig))
	viper.WatchConfig()
}

func updateServiceConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	for _, f := range onConfigChange {
		f()
	}

	return nil
}

func setLogger() {
	log.SetDefaultLogger(
		log.WithLevel(logLevel.Value()),
		log.WithTimeFormat("2006-01-02T15:04:05Z07:00"),
		log.SetStacktrace(addTrace.Value()),
		log.WithName(cliName),
		log.WithFormat(logFormat.Value()),
	)
	logger = log.Default.New(log.WithName(cliName))
}
