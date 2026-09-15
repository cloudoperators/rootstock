// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"
	"time"

	"github.com/SAP/rootstock/pkg/log"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	origRunServer := runServer
	origChildLogger := childLogger
	origLogger := logger
	origRootKey := cfgRootKey
	t.Cleanup(func() {
		runServer = origRunServer
		childLogger = origChildLogger
		logger = origLogger
		cfgRootKey = origRootKey
		viper.Reset()
	})
	cfgRootKey = ""
	viper.Reset()

	mockLog := log.NewMockLogger()
	logger = mockLog

	serverCalled := make(chan struct{})
	var capturedPort int
	runServer = func(port int, l log.MinInterface) {
		capturedPort = port
		close(serverCalled)
		select {} // block until the test binary exits
	}

	var capturedChildName string
	childLogger = func(l log.FullInterface, name string) log.MinInterface {
		capturedChildName = name
		return l.New(log.WithName(name))
	}

	cmd := run()
	go cmd.Run(cmd, nil)

	select {
	case <-serverCalled:
	case <-time.After(time.Second):
		t.Fatal("runServer was not called within timeout")
	}

	assert.Equal(t, 9220, capturedPort)
	assert.Equal(t, "run", capturedChildName)

	mockLog.SortByType()
	assert.True(t, mockLog.Contains(log.InfoLvl, "starting "+cliName+" api"))
	assert.True(t, mockLog.Contains(log.InfoLvl, "rootstock starting up"))
}
