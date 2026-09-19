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
	t.Parallel()

	selectedTest := ""
	// "calls updateFunc with updated content on file change"

	tests := []struct {
		name               string
		fileContentAtStart string
		fileContentUpdates []string
		updateErr          error
		wantErr            string
	}{
		{
			name:               "calls updateFunc with file content on init",
			fileContentAtStart: "config: value",
		},
		{
			name:    "returns error when file does not exist",
			wantErr: "no such file or directory",
		},
		{
			name:               "propagates updateFunc error",
			fileContentAtStart: "some content",
			updateErr:          errors.New("update failed"),
			wantErr:            "failed to update clients on config file change",
		},
		{
			name:               "calls updateFunc with updated content on file change",
			fileContentAtStart: "value: initial",
			fileContentUpdates: []string{"value: updated"},
		},
	}

	for _, tt := range tests {
		if selectedTest != "" && tt.name != selectedTest {
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filePath, updateFn,
				expectedContents, actualContents,
				lock := setupFileConfig(t, tt.fileContentAtStart, tt.updateErr)

			err := InitFileConfig(filePath, updateFn)

			checkErr(t, tt.wantErr, err)

			for _, u := range tt.fileContentUpdates {
				require.NoError(t, os.WriteFile(filePath, []byte(u), 0o600))
				*expectedContents = append(*expectedContents, u)

				time.Sleep(100 * time.Millisecond)
			}

			assert.Eventually(t, func() bool {
				lock.Lock()
				defer lock.Unlock()

				return assert.Equal(t, expectedContents, actualContents,
					"expected file content and acutal content do not match",
				)
			},
				2*time.Second, 200*time.Millisecond,
				"updateFunc was not called with updated content",
			)
		})
	}
}

func checkErr(t *testing.T, want string, got error) {
	t.Helper()

	if want != "" {
		require.Error(t, got)
		assert.Contains(t, got.Error(), want)

		return
	}

	require.NoError(t, got)
}

func setupFileConfig(t *testing.T, contentAtStart string, updateErr error) (
	filePath string, updateFunc func(c []byte) error,
	expectedContents, actualContents *[]string, lock *sync.Mutex,
) {
	t.Helper()

	if contentAtStart == "" {
		filePath = filepath.Join(t.TempDir(), "nonexistent.yaml")
		expectedContents = &[]string{}
	} else {
		filePath = filepath.Join(t.TempDir(), "config.yaml")
		expectedContents = &[]string{contentAtStart}
		require.NoError(t, os.WriteFile(filePath, []byte(contentAtStart), 0o600))
	}

	lock = &sync.Mutex{}
	actualContents = &[]string{}

	updateFunc = func(content []byte) error {
		lock.Lock()
		defer lock.Unlock()

		*actualContents = append(*actualContents, string(content))

		return updateErr
	}

	return filePath, updateFunc, expectedContents, actualContents, lock
}

func TestOnFileChange(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
