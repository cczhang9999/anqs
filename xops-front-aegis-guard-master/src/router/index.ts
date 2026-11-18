import { createRouter, createWebHashHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import type { App } from 'vue'
import { Layout } from '@/utils/routerHelper'
import { NO_RESET_WHITE_LIST } from '@/constants'

export const constantRouterMap = [
  {
    path: "/redirect",
    component: Layout,
    meta: { hidden: true },
    children: [
      {
        path: "/redirect/:path(.*)",
        component: () => import("@/views/Redirect/Redirect.vue")
      },
    ],
  },
  {
    path: '/404',
    component: () => import('@/views/Error/404.vue'),
    name: 'NoFind',
    meta: {
      hidden: true,
      title: '404',
      noTagsView: true
    }
  }
]

export const asyncRouterMap: AppRouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('@/views/Aegis/AegisLayout.vue'),
    name: 'aegis-guard-index',
    redirect: '/aegis/alerts',
    meta: { title: '安骑士告警分组系统', alwaysShow: false },
    children: [
      {
        path: 'aegis/alerts',
        component: () => import('@/views/Aegis/Alerts.vue'),
        name: 'aegis-alerts',
        meta: { title: '告警确认', noCache: true, icon: 'vi-ant-design:alert-filled', affix: true }
      },
      {
        path: 'aegis/devices',
        component: () => import('@/views/Aegis/Devices.vue'),
        name: 'aegis-devices',
        meta: { title: '主机分组映射', noCache: true, icon: 'vi-ant-design:cluster-outlined' }
      },
      {
        path: 'aegis/group-rules',
        component: () => import('@/views/Aegis/GroupRules.vue'),
        name: 'aegis-group-rules',
        meta: { title: '分组规则', noCache: true, icon: 'vi-ant-design:partition-outlined' }
      },
      {
        path: 'aegis/departments',
        component: () => import('@/views/Aegis/Departments.vue'),
        name: 'aegis-departments',
        meta: { title: '部门告警群配置', noCache: true, icon: 'vi-ant-design:team-outlined' }
      }
    ]
  }
]

// 本地开发直接注册业务路由，避免首次直达 /aegis/* 时先命中 404，
// 之后再由权限守卫重复添加动态路由导致页面闪烁或打不开。
const initialRoutes = import.meta.env.VITE_BYPASS_SSO === 'true'
  ? (constantRouterMap as RouteRecordRaw[]).concat(asyncRouterMap as RouteRecordRaw[])
  : constantRouterMap as RouteRecordRaw[]

const router = createRouter({
  history: createWebHashHistory(),
  strict: true,
  routes: initialRoutes,
  scrollBehavior: () => ({ left: 0, top: 0 })
})

export const resetRouter = (): void => {
  router.getRoutes().forEach((route) => {
    const { name } = route
    if (name && !NO_RESET_WHITE_LIST.includes(name as string)) {
      router.hasRoute(name) && router.removeRoute(name)
    }
  })
}

export const setupRouter = (app: App<Element>) => {
  app.use(router)
}

export default router
