package slice

import (
	"fmt"
	"reflect"
)

func Reverse(s interface{}) {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Slice {
		fmt.Println("Input is not a slice")
		return
	}
	length := v.Len()
	swap := reflect.Swapper(s)
	for i, j := 0, length-1; i < j; i, j = i+1, j-1 {
		swap(i, j)
	}
}
