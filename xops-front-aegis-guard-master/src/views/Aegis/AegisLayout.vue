<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Icon } from '@/components/Icon'

const route = useRoute()

const alertMenus = [
  { path: '/aegis/alerts', title: '告警确认', icon: 'vi-ant-design:alert-filled' }
]

// 左侧「配置」分组：主机分组映射 → 分组规则 → 部门告警群配置。
const configMenus = [
  { path: '/aegis/devices', title: '主机分组映射', icon: 'vi-ant-design:cluster-outlined' },
  { path: '/aegis/group-rules', title: '分组规则', icon: 'vi-ant-design:partition-outlined' },
  { path: '/aegis/departments', title: '部门告警群配置', icon: 'vi-ant-design:team-outlined' }
]

const menus = [...alertMenus, ...configMenus]

const currentMenu = computed(() => menus.find((item) => route.path === item.path) || menus[0])

watch(currentMenu, (menu) => { document.title = `安骑士告警分组系统 - ${menu.title}` }, { immediate: true })
</script>

<template>
  <div class="aegis-shell">
    <aside class="aegis-sidebar">
      <router-link to="/aegis/alerts" class="aegis-logo">
        <img src="/aegis-shield-alert.svg" alt="安骑士告警分组系统" />
        <span>安骑士告警分组系统</span>
      </router-link>

      <div class="aegis-menu-group">
        <div class="aegis-menu-label">告警</div>
        <router-link
          v-for="item in alertMenus"
          :key="item.path"
          :to="item.path"
          class="aegis-menu-item"
          :class="{ active: route.path === item.path }"
        >
          <Icon :icon="item.icon" :size="17" />
          <span>{{ item.title }}</span>
        </router-link>
      </div>

      <div class="aegis-menu-group">
        <div class="aegis-menu-label">配置</div>
        <router-link
          v-for="item in configMenus"
          :key="item.path"
          :to="item.path"
          class="aegis-menu-item"
          :class="{ active: route.path === item.path }"
        >
          <Icon :icon="item.icon" :size="17" />
          <span>{{ item.title }}</span>
        </router-link>
      </div>
    </aside>

    <section class="aegis-main">
      <header class="aegis-topbar">
        <span class="topbar-name">安骑士告警分组系统</span>
        <span class="topbar-separator">/</span>
        <strong>{{ currentMenu.title }}</strong>
      </header>
      <main class="aegis-content">
        <RouterView />
      </main>
    </section>
  </div>
</template>

<style scoped>
.aegis-shell {
  --aegis-teal: #20b2a6;
  --aegis-teal-dark: #1a9c92;
  --aegis-text: #3b4451;
  --aegis-sub: #8a94a6;
  --aegis-border: #e4e7ed;
  --aegis-sidebar: #2b3648;
  --aegis-sidebar-dark: #232c3b;
  --aegis-bg: #f3f5f8;
  display: flex;
  width: 100%;
  height: 100vh;
  overflow: hidden;
  background: var(--aegis-bg);
  color: var(--aegis-text);
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
  font-size: 14px;
}

.aegis-sidebar {
  width: 216px;
  flex: 0 0 216px;
  height: 100%;
  overflow-y: auto;
  background: var(--aegis-sidebar);
  color: #c7cedb;
}

.aegis-logo {
  display: flex;
  align-items: center;
  height: 52px;
  padding: 0 16px;
  gap: 9px;
  color: #fff;
  background: var(--aegis-sidebar-dark);
  text-decoration: none;
  font-size: 15px;
  font-weight: 600;
  white-space: nowrap;
}

.aegis-logo img { width: 27px; height: 27px; object-fit: contain; }
.aegis-menu-group { padding: 19px 10px 0; }
.aegis-menu-label { padding: 0 12px 8px; color: #8290a4; font-size: 12px; }
.aegis-menu-item { display: flex; align-items: center; gap: 10px; height: 40px; padding: 0 12px; border-radius: 5px; color: #c7cedb; text-decoration: none; transition: background .2s, color .2s; }
.aegis-menu-item:hover { color: #fff; background: rgba(255, 255, 255, .08); }
.aegis-menu-item.active { color: #fff; background: var(--aegis-teal); }
.aegis-menu-item :deep(.v-admin-icon) { flex: 0 0 auto; }

.aegis-main { display: flex; flex: 1; min-width: 0; flex-direction: column; }
.aegis-topbar { display: flex; align-items: center; height: 52px; padding: 0 22px; gap: 10px; background: #fff; border-bottom: 1px solid var(--aegis-border); color: var(--aegis-sub); font-size: 13px; }
.aegis-topbar strong { color: var(--aegis-text); font-weight: 600; }
.topbar-name { color: #687386; }
.topbar-separator { color: #c2c9d2; }
.aegis-content { flex: 1; min-height: 0; overflow: auto; background: var(--aegis-bg); }

@media (max-width: 720px) {
  .aegis-sidebar { width: 190px; flex-basis: 190px; }
  .aegis-logo { padding: 0 12px; font-size: 13px; }
  .aegis-topbar { padding: 0 14px; }
}
</style>
