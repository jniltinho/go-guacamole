import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

import { useAuth } from '../stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    perm?: string
    title?: string
    bare?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/dashboard' },
  { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true, bare: true } },
  {
    path: '/dashboard',
    component: () => import('../views/DashboardView.vue'),
    meta: { title: 'nav.dashboard' },
  },
  {
    path: '/connections',
    component: () => import('../views/ConnectionsView.vue'),
    meta: { title: 'nav.connections' },
  },
  {
    path: '/connections/new',
    component: () => import('../views/ConnectionEditView.vue'),
    meta: { title: 'connections.create', perm: 'CREATE_CONNECTION' },
  },
  {
    path: '/connections/:id/edit',
    component: () => import('../views/ConnectionEditView.vue'),
    meta: { title: 'connections.edit' },
  },
  {
    path: '/sessions/:connectionId/client',
    component: () => import('../views/ClientView.vue'),
    meta: { bare: true },
  },
  {
    path: '/sessions',
    component: () => import('../views/SessionsView.vue'),
    meta: { title: 'nav.sessions' },
  },
  {
    path: '/users',
    component: () => import('../views/UsersView.vue'),
    meta: { title: 'nav.users', perm: 'ADMINISTER' },
  },
  // the new-user form is a dialog on the list
  { path: '/users/new', redirect: { path: '/users', query: { new: '' } } },
  {
    path: '/users/:id',
    component: () => import('../views/UserEditView.vue'),
    meta: { title: 'nav.users', perm: 'ADMINISTER' },
  },
  {
    path: '/groups',
    component: () => import('../views/GroupsView.vue'),
    meta: { title: 'nav.groups', perm: 'ADMINISTER' },
  },
  {
    path: '/groups/:id',
    component: () => import('../views/GroupEditView.vue'),
    meta: { title: 'nav.groups', perm: 'ADMINISTER' },
  },
  {
    path: '/connection-groups',
    component: () => import('../views/ConnectionGroupsView.vue'),
    meta: { title: 'nav.connectionGroups', perm: 'CREATE_CONNECTION' },
  },
  {
    path: '/permissions',
    component: () => import('../views/PermissionsView.vue'),
    meta: { title: 'nav.permissions', perm: 'ADMINISTER' },
  },
  {
    path: '/audit',
    component: () => import('../views/AuditView.vue'),
    meta: { title: 'nav.audit', perm: 'AUDIT' },
  },
  {
    path: '/recording',
    component: () => import('../views/RecordingSettingsView.vue'),
    meta: { title: 'nav.recording', perm: 'ADMINISTER' },
  },
  {
    path: '/authentication',
    component: () => import('../views/AuthSettingsView.vue'),
    meta: { title: 'nav.authentication', perm: 'ADMINISTER' },
  },
  {
    path: '/settings',
    component: () => import('../views/SettingsView.vue'),
    meta: { title: 'nav.settings' },
  },
  { path: '/403', component: () => import('../views/ForbiddenView.vue'), meta: { title: 'pages.forbidden' } },
  {
    path: '/:pathMatch(.*)*',
    component: () => import('../views/NotFoundView.vue'),
    meta: { title: 'pages.notFound' },
  },
]

export const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  const auth = useAuth()
  if (!auth.loaded) await auth.load()
  if (to.meta.public) return auth.me && to.path === '/login' ? '/dashboard' : true
  if (!auth.me) return { path: '/login', query: to.fullPath !== '/dashboard' ? { next: to.fullPath } : {} }
  if (to.meta.perm && !auth.has(to.meta.perm)) return '/403'
  return true
})
