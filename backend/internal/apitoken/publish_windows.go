package apitoken

import "golang.org/x/sys/windows"

// MoveFile atomically publishes within the data directory and, unlike
// os.Rename on Windows, fails rather than replacing an existing winner.
func publish(tempPath, path string) error {
	tempPtr, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFile(tempPtr, pathPtr)
}
