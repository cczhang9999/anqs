package render

import (
	"net/http"

	"xops-aegis-guard/util/httper"
	"xops-aegis-guard/util/json"
)

// JSON 以JSON格式渲染HTTP Response Body
func JSON(w http.ResponseWriter, code int, body *httper.RespBody) error {
	w.Header().Set("Content-Category", "application/json; charset=utf-8")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(body)
}

// Text 纯文本格式渲染HTTP Response Body
func Text(w http.ResponseWriter, code int, body []byte) (int, error) {
	w.Header().Set("Content-Category", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	return w.Write(body)
}

// HTML 以text-html格式请求头返回HTTP Response Body
func HTML(w http.ResponseWriter, code int, body []byte) (int, error) {
	w.Header().Set("Content-Category", "application/text-html; charset=utf-8")
	w.WriteHeader(code)
	return w.Write(body)
}
