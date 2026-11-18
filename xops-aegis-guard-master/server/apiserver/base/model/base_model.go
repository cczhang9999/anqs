package model

import (
	"database/sql/driver"
	"errors"
	"time"

	jsonutil "xops-aegis-guard/util/json"

	"gorm.io/gorm"
)

type Idmodel struct {
	ID uint `json:"id" gorm:"type:bigint;primaryKey;autoIncrement"` // ID
}

type CreateModel struct {
	Idmodel
	CreatedAt *time.Time `json:"created_at" gorm:"<-:create;type:timestamp;comment:创建时间"` // 创建时间
}

type TimeModel struct {
	CreateModel
	UpdatedAt *time.Time `json:"updated_at" gorm:"type:timestamp;comment:更新时间;default:null"` // 更新时间
}

type BaseModel struct {
	TimeModel
	DeletedAt gorm.DeletedAt `json:"-" gorm:"type:timestamp;comment:删除时间"` // 删除时间
}

// JSON 以 JSON 列保存结构化数据，写法与 port-check 中的数组类型一致，
// 通过 Scanner/Valuer 负责数据库读写，通过 JSON 方法负责接口编解码。
type JSON []byte

func validJSON(data []byte) error {
	var value interface{}
	if err := jsonutil.Unmarshal(data, &value); err != nil {
		return errors.New("invalid JSON")
	}
	return nil
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("failed to scan JSON")
	}
	if err := validJSON(data); err != nil {
		return err
	}
	*j = append((*j)[:0], data...)
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if err := validJSON(j); err != nil {
		return nil, err
	}
	return []byte(j), nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	if err := validJSON(j); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if err := validJSON(data); err != nil {
		return err
	}
	*j = append((*j)[:0], data...)
	return nil
}

// StringJSON 表示 JSON 字符串值，区别于 JSON 对象/数组。
type StringJSON string

func (j *StringJSON) Scan(value interface{}) error {
	data, err := jsonBytes(value)
	if err != nil {
		return err
	}
	return j.UnmarshalJSON(data)
}

func (j StringJSON) Value() (driver.Value, error) {
	return jsonutil.Marshal(string(j))
}

func (j StringJSON) MarshalJSON() ([]byte, error) {
	return jsonutil.Marshal(string(j))
}

func (j *StringJSON) UnmarshalJSON(data []byte) error {
	var value string
	if err := jsonutil.Unmarshal(data, &value); err != nil {
		return err
	}
	*j = StringJSON(value)
	return nil
}

// IntJSON 表示 JSON 整数值，区别于 JSON 字符串值。
type IntJSON int

func (j *IntJSON) Scan(value interface{}) error {
	data, err := jsonBytes(value)
	if err != nil {
		return err
	}
	return j.UnmarshalJSON(data)
}

func (j IntJSON) Value() (driver.Value, error) {
	return jsonutil.Marshal(int(j))
}

func (j IntJSON) MarshalJSON() ([]byte, error) {
	return jsonutil.Marshal(int(j))
}

func (j *IntJSON) UnmarshalJSON(data []byte) error {
	var value int
	if err := jsonutil.Unmarshal(data, &value); err != nil {
		return err
	}
	*j = IntJSON(value)
	return nil
}

func jsonBytes(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, errors.New("failed to scan JSON")
	}
}
