package http

import (
	"crypto/tls"
	"net/http"
	"time"
	"xops-aegis-guard/logger"
)

func NewCertClient() *HttpClient {
	httpcli := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}}
	c := &HttpClient{
		client:  httpcli,
		headers: make(http.Header),
	}
	return c
}
func (c *HttpClient) GetCertTime(url string) (certTime *CertTime, err error) {
	resp, err := c.client.Get(url)
	if err != nil {
		logger.Debug("Error fetching URL: ", err)
		return
	}
	// 获取连接的 TLS 信息
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		logger.Debug("No TLS certificates found")
		return
	}
	// 获取第一个证书
	cert := resp.TLS.PeerCertificates[0]
	certTime = &CertTime{
		StartTime:    cert.NotBefore,
		EndTime:      cert.NotAfter,
		StartTimeStr: cert.NotBefore.Format(time.DateTime),
		EndTimeStr:   cert.NotAfter.Format(time.DateTime),
	}
	return
}

type CertTime struct {
	StartTime    time.Time
	EndTime      time.Time
	StartTimeStr string
	EndTimeStr   string
}
