package app

import (
	"fmt"
	"os"
	"syscall"
)

// Keep the lock file in place: unlinking it would allow two different inodes
// to be locked simultaneously. Closing the descriptor releases the lock.
func acquireInstance(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开应用锁失败：%w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("hello-tsnet 已在运行，或无法锁定数据目录：%w", err)
	}
	return file, nil
}
