import type { App } from 'vue'

// 需要全局引入一些组件，如ElScrollbar，不然一些下拉项样式有问题
import {
  ElBadge,
  ElButton,
  ElCard,
  ElCheckbox,
  ElDatePicker,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElInputNumber,
  ElLoading,
  ElOption,
  ElPagination,
  ElProgress,
  ElScrollbar,
  ElSelect,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTag,
  ElTimePicker,
  ElTooltip
} from 'element-plus'

const plugins = [ElLoading]

const components = [
  ElBadge,
  ElButton,
  ElCard,
  ElCheckbox,
  ElDatePicker,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElInputNumber,
  ElOption,
  ElPagination,
  ElProgress,
  ElScrollbar,
  ElSelect,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTag,
  ElTimePicker,
  ElTooltip
]

export const setupElementPlus = (app: App<Element>) => {
  plugins.forEach((plugin) => {
    app.use(plugin)
  })

  components.forEach((component) => {
    app.component(component.name!, component)
  })

  // 为了开发环境启动更快，一次性引入所有样式
  if (import.meta.env.VITE_USE_ALL_ELEMENT_PLUS_STYLE === 'true') {
    import('element-plus/dist/index.css')
    return
  }
}
