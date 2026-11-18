import router from './router'
import { useAppStoreWithOut } from '@/store/modules/app'
import type { RouteRecordRaw } from 'vue-router'
import { useTitle } from '@/hooks/web/useTitle'
import { useNProgress } from '@/hooks/web/useNProgress'
import { usePermissionStoreWithOut } from '@/store/modules/permission'
import { usePageLoading } from '@/hooks/web/usePageLoading'
import { CookieUtils } from '@/utils/CookieUtils'

const { start, done } = useNProgress()

const { loadStart, loadDone } = usePageLoading()
const getSSoUrl = ()=> {
  const { protocol, hostname } = window.location;
  const parts = hostname.split(".");
  const tenant = parts[1];
  if (import.meta.env.VITE_AUTH == "auth_ceshi") {
    return `${protocol}//sso-uat.${tenant}.com`;
  } else if (import.meta.env.VITE_AUTH == "auth_pro") {
    return `${protocol}//sso.${tenant}.com`;
  } else {
    return import.meta.env.VITE_HERF_DOMAIN
  }
}
router.beforeEach(async (to, from, next) => {
  const bypassSSO = import.meta.env.VITE_BYPASS_SSO === 'true'

  if (!bypassSSO && !CookieUtils.get(import.meta.env.VITE_AUTH)) {
    window.location.href =getSSoUrl()
    return
  }
  start()
  loadStart()
  const permissionStore = usePermissionStoreWithOut()
  if (to.path === '/login') {
    next({ path: '/' })
  } else {
    if (permissionStore.getIsAddRouters) {
      next()
      return
    }

    // 本地开发可跳过 SSO，直接读取静态路由表
    await permissionStore.generateRoutes(bypassSSO ? 'static' : 'server')
    // console.log(permissionStore.getAddRouters,'permissionStore')
    permissionStore.getAddRouters.forEach((route) => {
      router.addRoute(route as unknown as RouteRecordRaw) // 动态添加可访问路由表
    })

    const redirectPath = from.query.redirect || to.path
    const redirect = decodeURIComponent(redirectPath as string)
    const nextData = to.path === redirect ? { ...to, replace: true } : { path: redirect }
    permissionStore.setIsAddRouters(true)
    next(nextData)
  }
})

router.afterEach((to) => {
  useTitle(to?.meta?.title as string)
  done() // 结束Progress
  loadDone()
})
