package iptool

import (
	"encoding/binary"
	"errors"
	"net"
)

func IPToInt(ipStr string) (ipNum uint32, err error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipNum, errors.New("IP 格式错误")
	}
	ip = ip.To4()
	if ip == nil {
		return ipNum, errors.New("IP 格式错误")
	}
	ipNum = binary.BigEndian.Uint32(ip)
	return
}
