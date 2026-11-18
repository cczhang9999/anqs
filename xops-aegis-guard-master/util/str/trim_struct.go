package str

import (
	"reflect"
	"strings"
	"xops-aegis-guard/util/reflecter"
)

// TrimStruct 去除结构体的空格
func TrimStruct(obj interface{}) {
	objT, objV := reflecter.GetPtrTypeValue(obj)

	switch objT.Kind() {
	case reflect.Struct:
		doTrimFields(objT, objV)
	case reflect.Slice:
		for i := 0; i < objV.Len(); i++ {
			doTrimFields(objV.Index(i).Type(), objV.Index(i))
		}
	}
}

// doTrimFields 执行去除结构体的空格
func doTrimFields(objT reflect.Type, objV reflect.Value) {
	for i := 0; i < objT.NumField(); i++ {
		fieldV := objV.Field(i)
		if !fieldV.CanSet() {
			continue
		}
		fieldT := objT.Field(i)
		kind := fieldT.Type.Kind()
		if fieldT.Anonymous && kind == reflect.Struct {
			doTrimFields(fieldT.Type, fieldV)
			continue
		}
		if !fieldV.CanInterface() {
			continue
		}
		switch kind {
		case reflect.String:
			// 使用 Value.String() 兼容自定义 string 类型（例如 StringJSON），
			// 不能直接把命名类型断言成 string。
			fieldV.SetString(strings.TrimSpace(fieldV.String()))
		case reflect.Slice:
			if fieldV.Type().Elem().Kind() == reflect.String {
				for i := 0; i < fieldV.Len(); i++ {
					item := fieldV.Index(i)
					if item.CanSet() {
						item.SetString(strings.TrimSpace(item.String()))
					}
				}
			}
		}
	}
}
