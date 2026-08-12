// Package folderpicker opens the native Windows directory picker.
package folderpicker

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"bluntcode/internal/process"
)

const pickerScript = "Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = 'Choose a workspace folder for Blunt Code'; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) }"

func Select(ctx context.Context) (path string, cancelled bool, err error) {
	if runtime.GOOS != "windows" {
		return "", false, fmt.Errorf("native folder selection is supported on Windows only")
	}
	result, err := process.Run(ctx, process.Request{Command: "powershell.exe", Args: []string{"-NoLogo", "-NoProfile", "-STA", "-Command", pickerScript}})
	if err != nil {
		return "", false, err
	}
	path = strings.TrimSpace(string(result.Stdout))
	return path, path == "", nil
}
