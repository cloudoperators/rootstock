// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/SAP/rootstock/pkg/log"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

const (
	logKeyFile = "file"
	logKeyID   = "changeID"
)

// InitFileConfig registers a live-reloading configuration from the given filePath.
//
// When the registered file changes, the provided updateFunc is called with the
// latest content of the configuration file.
// The updateFunc is also called once during the initialisation.
func InitFileConfig(
	filePath string, updateFunc func(content []byte) error,
) error {
	viperCfg := viper.New()
	viperCfg.SetConfigFile(filePath)

	viperCfg.OnConfigChange(
		onFileChange(func() error { return updateClient(filePath, updateFunc) }))
	viperCfg.WatchConfig()

	return updateClient(filePath, updateFunc)
}

func onFileChange(onChange func() error) func(in fsnotify.Event) {
	return func(e fsnotify.Event) {
		changeID := uuid.New()
		logger.Info("config file change - reloading", logKeyFile, e.Name, logKeyID, changeID)

		switch e.Op {
		case fsnotify.Create, fsnotify.Write:
			if err := onChange(); err != nil {
				logger.Error("onChange function call failed", logKeyFile, e.Name, logKeyID, changeID, log.KeyError, err)
			}

			logger.Debug("config file change - complete", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		case fsnotify.Remove, fsnotify.Rename, fsnotify.Chmod:
			logger.Debug("not implemented by viper", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		default:
			logger.Warn("case not handled by viper", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		}
	}
}

func updateClient(filePath string, updateFunc func([]byte) error) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file %q - %w", filePath, err)
	}

	if err := updateFunc(content); err != nil {
		return fmt.Errorf("failed to update clients on config file change - %w", err)
	}

	return nil
}
