package json

import (
	"io"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// RawMessage 保持与 encoding/json 相同的传输表示，同时允许调用方使用已配置的
// jsoniter 实现。
type RawMessage = jsoniter.RawMessage

// Number 保持与 encoding/json.Number 相同的使用方式。
type Number = jsoniter.Number

func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
func NewEncoder(writer io.Writer) *jsoniter.Encoder {
	return json.NewEncoder(writer)
}

func NewDecoder(reader io.Reader) *jsoniter.Decoder {
	return json.NewDecoder(reader)
}

// MarshalIndent 接受标准的 prefix、indent 参数，同时兼容项目历史上的单参数调用。
func MarshalIndent(v interface{}, options ...string) ([]byte, error) {
	prefix, indent := "", "  "
	if len(options) > 0 {
		prefix = options[0]
	}
	if len(options) > 1 {
		indent = options[1]
	}
	return json.MarshalIndent(v, prefix, indent)
}
