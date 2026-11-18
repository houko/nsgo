//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package main

import "os"

// tryLockFile takes no lock where the system offers none the tool uses: checks there do not wait for each other.
func tryLockFile(*os.File) (bool, error) { return true, nil }

// unlockFile does nothing, as tryLockFile took no lock.
func unlockFile(*os.File) error { return nil }
