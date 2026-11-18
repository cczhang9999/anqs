package api

type BaseRespBody struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type IntIDResp struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Content struct {
		ID int `json:"id"`
	} `json:"content"`
}

type AffectedResp struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Content struct {
		Affected int `json:"affected"`
	} `json:"content"`
}
