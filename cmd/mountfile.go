// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/cloudoperators/rootstock/pkg/log"
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
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	if stat.IsDir() {
		return fmt.Errorf("%q must be a file but is a directory", filePath)
	}

	viperCfg := viper.New()
	viperCfg.SetConfigFile(filePath)

	var (
		mu          sync.Mutex
		lastContent []byte
	)

	update := func() error {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read config file %q - %w", filePath, err)
		}

		mu.Lock()
		defer mu.Unlock()

		if bytes.Equal(content, lastContent) {
			return nil
		}

		if err := updateFunc(content); err != nil {
			return fmt.Errorf("failed to update clients on config file change - %w", err)
		}

		lastContent = content

		return nil
	}

	viperCfg.OnConfigChange(onFileChange(update))
	viperCfg.WatchConfig()

	return update()
}

func onFileChange(onChange func() error) func(in fsnotify.Event) {
	return func(e fsnotify.Event) {
		changeID := uuid.New()
		logger.Info("config file change - reloading", logKeyFile, e.Name, logKeyID, changeID)

		switch {
		case e.Has(fsnotify.Create) || e.Has(fsnotify.Write):
			if err := onChange(); err != nil {
				logger.Error("onChange function call failed", logKeyFile, e.Name, logKeyID, changeID, log.KeyError, err)
			}

			logger.Debug("config file change - complete", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		case e.Has(fsnotify.Remove) || e.Has(fsnotify.Rename) || e.Has(fsnotify.Chmod):
			logger.Debug("not implemented by viper", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		default:
			logger.Warn("case not handled by viper", "event", e.Op, logKeyFile, e.Name, logKeyID, changeID)
		}
	}
}
