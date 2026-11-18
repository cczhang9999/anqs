import { defineStore } from 'pinia'
import { asyncRouterMap, constantRouterMap } from '@/router'
import {
  generateRoutesByFrontEnd,
  generateRoutesByServer,
  flatMultiLevelRoutes
} from '@/utils/routerHelper'
import { store } from '../index'
import { cloneDeep } from 'lodash-es'
import { authApi } from '@/api/user'

export interface PermissionState {
  routers: AppRouteRecordRaw[]
  addRouters: AppRouteRecordRaw[]
  isAddRouters: boolean
  menuTabRouters: AppRouteRecordRaw[]
  userId: string
  userName: string
  componentList: Record<string, boolean> | string[]
}

export const usePermissionStore = defineStore('permission', {
  state: (): PermissionState => ({
    routers: [],
    addRouters: [],
    isAddRouters: false,
    menuTabRouters: [],
    userId: '',
    userName: '',
    componentList: {}
  }),
  getters: {
    getRouters(): AppRouteRecordRaw[] {
      return this.routers
    },
    getAddRouters(): AppRouteRecordRaw[] {
      return flatMultiLevelRoutes(cloneDeep(this.addRouters))
    },
    getIsAddRouters(): boolean {
      return this.isAddRouters
    },
    getMenuTabRouters(): AppRouteRecordRaw[] {
      return this.menuTabRouters
    }
  },
  actions: {
    generateRoutes(
      type: 'server' | 'frontEnd' | 'static',
      routers?: AppCustomRouteRecordRaw[] | string[]
    ): Promise<unknown> {
      return new Promise<void>(async (resolve) => {
        let routerMap: AppRouteRecordRaw[] = []
        if (type === 'server') {
          // 模拟后端过滤菜单
          let arr = await this.getRoutersArr()
          routerMap = generateRoutesByServer(arr as AppCustomRouteRecordRaw[])
        } else if (type === 'frontEnd') {
          // 模拟前端过滤菜单
          routerMap = generateRoutesByFrontEnd(cloneDeep(asyncRouterMap), routers as string[])
        } else {
          // 直接读取静态路由表
          routerMap = cloneDeep(asyncRouterMap)
        }

        // 动态路由，404一定要放到最后面
        this.addRouters = routerMap.concat([
          {
            path: '/:path(.*)*',
            redirect: '/404',
            name: '404Page',
            meta: {
              hidden: true,
              breadcrumb: false
            }
          }
        ])
        // 渲染菜单的所有路由
        this.routers = cloneDeep(constantRouterMap).concat(routerMap)
        resolve()
      })
    },
    setIsAddRouters(state: boolean): void {
      this.isAddRouters = state
    },
    setMenuTabRouters(routers: AppRouteRecordRaw[]): void {
      this.menuTabRouters = routers
    },
    // 获取用户信息
    getAuthorization() {
      return new Promise(async (resolve, reject) => {
        try {
          let res = await authApi()
          this.userId = res.user_id
          this.userName = res.username
          this.componentList = res?.data?.rules?.component || {}
          resolve(res?.data?.rules?.page || {})
        } catch (error) {
          reject(error)
        }
      })
    },
    async getRoutersArr() {
      const rules = await this.getAuthorization()
      let routerList = []
      routerList = this.getAccesRouterList(asyncRouterMap, rules)
      return routerList
    },
    getAccesRouterList(routes, rules) {
      return routes.filter((item) => {
        if (rules[item.name]) {
          if (item.children) item.children = this.getAccesRouterList(item.children, rules)
          return true
        } else return false
      })
    }
  },
  persist: {
    paths: ['routers', 'addRouters', 'menuTabRouters']
  }
})

export const usePermissionStoreWithOut = () => {
  return usePermissionStore(store)
}
