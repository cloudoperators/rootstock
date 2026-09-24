// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	cliCTX       context.Context
	shutdownLock sync.Mutex

	gracePeriod time.Duration = 1 * time.Second

	// package-specific functions that should be called for graceful shutdown and
	// log.Sync functions for additional loggers.
	stopFuncs  []func()
	stopAction = func(exitCode int) { os.Exit(exitCode) }
)

// init sets up a context that will be canceled on OS signals.
//
//nolint:gochecknoinits // shutdown is a global package and init is used to set up the context and signal handling.
func init() {
	var stop context.CancelFunc
	//nolint:fatcontext // this context is used globally to handle shutdown signals
	cliCTX, stop = signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	)

	stopFuncs = []func(){stop}
}

func GetContext() context.Context {
	return cliCTX
}

// AddStopFunction adds a function to be called when the context is canceled.
// The function will be called in the order they were added.
func AddStopFunction(f func()) {
	shutdownLock.Lock()
	defer shutdownLock.Unlock()

	stopFuncs = append(stopFuncs, f)
}

// StopNowWith will invoke the stop function to cancel the context.
// It will panic if stop is nil.
func StopNowWith(exitCode int) {
	shutdownLock.Lock()
	defer shutdownLock.Unlock()

	for _, f := range stopFuncs {
		f()
	}

	time.Sleep(gracePeriod)

	stopAction(exitCode)
}

func StopNow() {
	StopNowWith(1)
}
