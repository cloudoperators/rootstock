// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitFileConfig(t *testing.T) {
	tests := []struct {
		name           string
		fileContent    []byte
		noFile         bool
		updateErr      error
		wantCalled     bool
		wantErr        string
		updatedContent []byte
	}{
		{
			name:        "calls updateFunc with file content on init",
			fileContent: []byte("config: value"),
			wantCalled:  true,
		},
		{
			name:       "returns error when file does not exist",
			noFile:     true,
			wantCalled: false,
			wantErr:    "failed to read config file",
		},
		{
			name:        "propagates updateFunc error",
			fileContent: []byte("some content"),
			updateErr:   errors.New("update failed"),
			wantCalled:  true,
			wantErr:     "failed to update clients on config file change",
		},
		{
			name:           "calls updateFunc with updated content on file change",
			fileContent:    []byte("value: initial"),
			wantCalled:     true,
			updatedContent: []byte("value: updated"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.noFile {
				filePath = filepath.Join(t.TempDir(), "nonexistent.yaml")
			} else {
				filePath = filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(filePath, tt.fileContent, 0o644))
			}

			var mu sync.Mutex
			var calls [][]byte
			updateFn := func(content []byte) error {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, content)
				return tt.updateErr
			}

			err := InitFileConfig(filePath, updateFn)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			mu.Lock()
			if tt.wantCalled {
				require.NotEmpty(t, calls)
				assert.Equal(t, tt.fileContent, calls[0])
			} else {
				assert.Empty(t, calls)
			}
			mu.Unlock()

			if tt.updatedContent != nil {
				require.NoError(t, os.WriteFile(filePath, tt.updatedContent, 0o644))
				assert.Eventually(t, func() bool {
					mu.Lock()
					defer mu.Unlock()
					if len(calls) < 2 {
						return false
					}
					return assert.Equal(t, tt.updatedContent, calls[len(calls)-1])
				}, 2*time.Second, 50*time.Millisecond, "updateFunc was not called with updated content")
			}
		})
	}
}

func TestOnFileChange(t *testing.T) {
	tests := []struct {
		name         string
		op           fsnotify.Op
		wantOnChange bool
		changeErr    error
	}{
		{
			name:         "Write event triggers onChange",
			op:           fsnotify.Write,
			wantOnChange: true,
		},
		{
			name:         "Create event triggers onChange",
			op:           fsnotify.Create,
			wantOnChange: true,
		},
		{
			name:         "Write event with onChange error does not panic",
			op:           fsnotify.Write,
			wantOnChange: true,
			changeErr:    errors.New("change error"),
		},
		{
			name:         "Remove event does not trigger onChange",
			op:           fsnotify.Remove,
			wantOnChange: false,
		},
		{
			name:         "Rename event does not trigger onChange",
			op:           fsnotify.Rename,
			wantOnChange: false,
		},
		{
			name:         "Chmod event does not trigger onChange",
			op:           fsnotify.Chmod,
			wantOnChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := onFileChange(func() error {
				called = true
				return tt.changeErr
			})

			handler(fsnotify.Event{Name: "config.yaml", Op: tt.op})

			assert.Equal(t, tt.wantOnChange, called)
		})
	}
}
