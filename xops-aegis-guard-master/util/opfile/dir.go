package opfile

import (
	"os"
)

// CreateDirIfNotExist 递归创建不存在的目录。
func CreateDirIfNotExist(dir string) error {
	if CheckPathIfNotExist(dir) {
		return nil
	}
	return os.MkdirAll(dir, os.ModePerm)
}

// CheckPathIfNotExist 判断路径是否存在，存在时返回 true。
func CheckPathIfNotExist(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

// RemoveFile 删除文件，无论文件是否存在。
func RemoveFile(path string) {
	_ = os.Remove(path)
}
