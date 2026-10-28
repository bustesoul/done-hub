//go:build unix

package audit

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockArchiveDirectory(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("审计归档目录正在由其他进程使用: %w", err)
	}
	return file, nil
}
