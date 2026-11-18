import type { App, Directive, DirectiveBinding } from 'vue'
import { usePermissionStoreWithOut } from '@/store/modules/permission'

export const hasComponentPermission = (value: string): boolean => {
  if (import.meta.env.VITE_BYPASS_SSO === 'true') return true
  if (!value) return false

  const rules = usePermissionStoreWithOut().componentList || {}
  if (Array.isArray(rules)) return rules.includes(value)
  return Boolean(rules[value])
}

function hasPermi(el: Element, binding: DirectiveBinding) {
  const value = binding.value
  if (!hasComponentPermission(value)) {
    el.parentNode?.removeChild(el)
  }
}
const mounted = (el: Element, binding: DirectiveBinding<any>) => {
  hasPermi(el, binding)
}

const permiDirective: Directive = {
  mounted
}

export const setupPermissionDirective = (app: App<Element>) => {
  app.directive('hasPermi', permiDirective)
}

export default permiDirective
