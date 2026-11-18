package pager

import (
	"errors"
	"reflect"
	"strconv"

	"gorm.io/gorm"
)

// Parse 将 HTTP 查询参数转换为统一分页参数。
func Parse(limitValue, offsetValue string) (limit, offset int) {
	limit, _ = strconv.Atoi(limitValue)
	offset, _ = strconv.Atoi(offsetValue)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// page: 当前页码
// pageSize: 每页条数
// rowsSlicePtr: 要求传入一个slice的指针类型 &[]model.AuthUser，否则出现错误
// rowElement: slice里面的单个元素，会将其中的非0值元素作为查询条件
// session: 为带有查询条件的session 可以携带一些特殊的查询条件 rowElement 无法做到的，如：like 等
// 如果查询总数为0则不会继续往后查询
// 会赋值传入的slice为查询结果，并返回查询总数total，或执行中出现的error错误
func Help(page, pageSize int, rowsSlicePtr interface{}, rowElement interface{}, db *gorm.DB) (count int64, err error) {
	// 保证分页数据合法性
	page, pageSize = valPageAndPageSize(page, pageSize)

	// 保证传入的是slice类型
	sliceValue := reflect.Indirect(reflect.ValueOf(rowsSlicePtr))
	if sliceValue.Kind() != reflect.Slice {
		return 0, errors.New("需要传入一个slice类型")
	}
	// 查询总数，如果总数为0，就不继续查询了
	err = db.Model(&rowElement).Count(&count).Error
	if err != nil || count == 0 {
		return
	}
	// 查询具体数据
	startNum := (page - 1) * pageSize
	err = db.Limit(pageSize).Offset(startNum).Find(rowsSlicePtr).Error
	return
}

// page: 当前页码
// pageSize: 每页条数
// rowsSlicePtr: 要求传入一个slice的指针类型 &[]model.AuthUser，否则出现错误
// rowElement: slice里面的单个元素，会将其中的非0值元素作为查询条件
// session: 为带有查询条件的session 可以携带一些特殊的查询条件 rowElement 无法做到的，如：like 等
// 如果查询总数为0则不会继续往后查询
// 会赋值传入的slice为查询结果，并返回查询总数total，或执行中出现的error错误
func HelpNoCount(page, pageSize int, rowsSlicePtr interface{}, rowElement interface{}, db *gorm.DB) (count int64, err error) {
	// 保证分页数据合法性
	page, pageSize = valPageAndPageSize(page, pageSize)

	// 保证传入的是slice类型
	sliceValue := reflect.Indirect(reflect.ValueOf(rowsSlicePtr))
	if sliceValue.Kind() != reflect.Slice {
		return 0, errors.New("需要传入一个slice类型")
	}
	// 查询具体数据
	startNum := (page - 1) * pageSize
	err = db.Limit(pageSize).Offset(startNum).Find(rowsSlicePtr).Error
	return
}
