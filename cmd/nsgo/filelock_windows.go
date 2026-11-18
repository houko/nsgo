package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	procLockFileEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	procUnlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x1
	lockfileExclusiveLock   = 0x2
	errorLockViolation      = syscall.Errno(33)
)

// fileLockRange is the byte locked: one past 4 GiB, far beyond what the file holds, as Windows' locks keep other processes from reading the bytes they cover.
func fileLockRange() *syscall.Overlapped {
	return &syscall.Overlapped{OffsetHigh: 1}
}

// tryLockFile takes f's exclusive lock without waiting, reporting false when another open file holds it. The system drops the lock when its process ends, however it ends.
func tryLockFile(f *os.File) (bool, error) {
	r, _, err := procLockFileEx.Call(f.Fd(), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, uintptr(unsafe.Pointer(fileLockRange())))
	if r != 0 {
		return true, nil
	}
	if errors.Is(err, errorLockViolation) {
		return false, nil
	}
	return false, err
}

// unlockFile releases f's lock.
func unlockFile(f *os.File) error {
	r, _, err := procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(fileLockRange())))
	if r == 0 {
		return err
	}
	return nil
}
