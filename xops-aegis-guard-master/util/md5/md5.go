package md5

import (
	"crypto/md5"
	"fmt"
)

func Md5(str string) (hex string) {
	hash := md5.New()
	hash.Write([]byte(str))
	digest := hash.Sum(nil)
	hex = fmt.Sprintf("%x", digest)
	return
}
