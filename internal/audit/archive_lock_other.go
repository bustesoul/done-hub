//go:build !unix

package audit

import (
	"fmt"
	"os"
)

func lockArchiveDirectory(string) (*os.File, error) {
	return nil, fmt.Errorf("审计归档模式需要支持 flock 的 Unix 宿主机")
}
