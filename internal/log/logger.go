// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// SPDX-FileCopyrightText: rootstock contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger wraps a logrus.Entry so every log line carries service context
type Logger struct {
	*logrus.Entry
}

func NewLogger(serviceName, env string) *Logger {
	l := logrus.New()
	l.SetOutput(os.Stdout)

	if env == "LOCAL" {
		l.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		l.SetLevel(logrus.DebugLevel)
	} else {
		l.SetFormatter(&logrus.JSONFormatter{})
		l.SetLevel(logrus.InfoLevel)
	}

	entry := l.WithFields(logrus.Fields{
		"service_name": serviceName,
		"service_env":  env,
	})

	return &Logger{Entry: entry}
}
