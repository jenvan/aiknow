/**
 * 后台应用外壳：登录态、客户端路由、侧边栏。
 *
 * 页面组件以 Alpine.data() 注册，模板写在 index.html 的 <template x-if> 中
 * ——这是 Alpine 的惯用法。不采用「JS 拼 HTML 字符串 + x-html」的方案：
 * x-html 插入的内容不会被 Alpine 处理，页面内的指令会全部失效。
 */

import { fetchSession, api } from './api.js';
import { registerDashboard } from './pages/dashboard.js';
import { registerArticles } from './pages/articles.js';
import { registerArticleEdit } from './pages/article-edit.js';
import { registerArticlePreview } from './pages/article-preview.js';
import { registerArticleNew } from './pages/article-new.js';
import { registerSearch } from './pages/search.js';
import { registerSettings } from './pages/settings.js';
import { registerApiKeys } from './pages/api-keys.js';
import { registerMcpDebug } from './pages/mcp-debug.js';
import { registerMcpLogs } from './pages/mcp-logs.js';
import { registerWeixin } from './pages/weixin.js';

// 路由表：先匹配者优先，故 /articles/new 必须排在 /articles/:id 之前，
// /articles/:id/preview 必须排在 /articles/:id 之前（否则 preview 会当成 id）。
const ROUTES = [
    { re: /^\/ui\/?$/,                    page: 'dashboard' },
    { re: /^\/ui\/dashboard\/?$/,         page: 'dashboard' },
    { re: /^\/ui\/articles\/new\/?$/,     page: 'article-new' },
    { re: /^\/ui\/articles\/([^/]+)\/preview\/?$/, page: 'article-preview' },
    { re: /^\/ui\/articles\/([^/]+)\/?$/, page: 'article-edit' },
    { re: /^\/ui\/articles\/?$/,          page: 'articles' },
    { re: /^\/ui\/search\/?$/,            page: 'search' },
    { re: /^\/ui\/settings\/api-keys\/?$/, page: 'api-keys' },
    { re: /^\/ui\/settings\/?$/,          page: 'settings' },
    { re: /^\/ui\/mcp-debug\/?$/,         page: 'mcp-debug' },
    { re: /^\/ui\/mcp-logs\/?$/,          page: 'mcp-logs' },
    { re: /^\/ui\/weixin\/?$/,            page: 'weixin' },
];

// 导航图标：inline-SVG 内层路径（heroicons outline 风格）。
// stroke 用 currentColor，让 active/inactive 只靠文字色切换即可高亮。
const ICONS = {
    dashboard: '<rect x="3" y="3" width="7.5" height="7.5" rx="1.5"/><rect x="13.5" y="3" width="7.5" height="7.5" rx="1.5"/><rect x="3" y="13.5" width="7.5" height="7.5" rx="1.5"/><rect x="13.5" y="13.5" width="7.5" height="7.5" rx="1.5"/>',
    articles:  '<path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/>',
    'article-new': '<rect x="3.5" y="3.5" width="17" height="17" rx="2"/><path d="M12 8.5v7M8.5 12h7"/>',
    search:    '<circle cx="11" cy="11" r="7"/><path d="m20.5 20.5-4.2-4.2"/>',
    'mcp-debug': '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/>',
    'mcp-logs': '<path d="M7 3h7l4 4v14H7z"/><path d="M14 3v4h4M10 12h5M10 16h5"/>',
    settings:  '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9 7 7M17 17l2.1 2.1M19.1 4.9 17 7M7 17l-2.1 2.1"/>',
    'api-keys':'<circle cx="7.5" cy="7.5" r="3.5"/><path d="m10.2 10.2 8.8 8.8M15 15l2.2-2.2M17.5 17.5l2-2"/>',
    weixin:   '<path d="M4 5.5h16v10H9.5L5 19.5z"/><path d="M9 10.5h.01M12.5 10.5h.01M16 10.5h.01"/>',
};

// 导航按功能域分组（展示顺序即渲染顺序）。URL/路由不变，只改侧栏展示。
const NAV_GROUPS = [
    { name: '概览', items: [
        { page: 'dashboard',   href: '/ui/dashboard',         label: '仪表盘',  icon: ICONS.dashboard },
    ]},
    { name: '内容', items: [
        { page: 'articles',    href: '/ui/articles',          label: '文章列表', icon: ICONS.articles },
        { page: 'article-new', href: '/ui/articles/new',      label: '录入文章', icon: ICONS['article-new'] },
    ]},
    { name: '检索 & 调试', items: [
        { page: 'search',    href: '/ui/search',          label: '检索测试', icon: ICONS.search },
        { page: 'mcp-debug', href: '/ui/mcp-debug',       label: 'MCP 调试', icon: ICONS['mcp-debug'] },
        { page: 'mcp-logs',  href: '/ui/mcp-logs',        label: 'MCP 日志', icon: ICONS['mcp-logs'] },
    ]},
    { name: '系统', items: [
        { page: 'api-keys',  href: '/ui/settings/api-keys', label: '密钥管理', icon: ICONS['api-keys'] },
        { page: 'settings',  href: '/ui/settings',        label: '系统配置', icon: ICONS.settings },
        { page: 'weixin',    href: '/ui/weixin',          label: '微信入口', icon: ICONS.weixin },
    ]},
];

// 平铺列表（供 resolve / pageTitle 复用）
const NAV_ITEMS = NAV_GROUPS.flatMap((g) => g.items);

// 各页标题（侧栏/header 显示；非导航页如文章编辑/预览单独映射）
const PAGE_TITLES = {
    'article-edit': '文章编辑',
    'article-preview': '文章预览',
};

function resolve(pathname) {
    for (const r of ROUTES) {
        const m = pathname.match(r.re);
        if (m) return { page: r.page, param: m[1] || '' };
    }
    return { page: 'dashboard', param: '' };
}

document.addEventListener('alpine:init', () => {
    const Alpine = window.Alpine;

    // 各页面组件自行注册
    registerDashboard(Alpine);
    registerArticles(Alpine);
    registerArticleEdit(Alpine);
    registerArticlePreview(Alpine);
    registerArticleNew(Alpine);
    registerSearch(Alpine);
    registerSettings(Alpine);
    registerApiKeys(Alpine);
    registerMcpDebug(Alpine);
    registerMcpLogs(Alpine);
    registerWeixin(Alpine);

    // 路由参数（如文章 ID）经全局 store 传给页面组件，
    // 避免把参数解析逻辑复制到每个页面
    Alpine.store('route', { param: '' });

    Alpine.data('shell', () => ({
        ready: false,
        isLoggedIn: false,
        username: '',
        currentPage: '',
        sidebarOpen: false,          // 响应式抽屉：窄屏侧栏展开态
        navGroups: NAV_GROUPS,

        /** 当前页在导航里的标题（非导航页如文章编辑用 PAGE_TITLES 兜底） */
        pageTitle() {
            if (PAGE_TITLES[this.currentPage]) return PAGE_TITLES[this.currentPage];
            const found = NAV_ITEMS.find((i) => i.page === this.currentPage);
            return found ? found.label : '仪表盘';
        },

        /** 切换并关闭抽屉（页内链接跳转后自动收起） */
        toggleSidebar() { this.sidebarOpen = !this.sidebarOpen; },
        closeSidebar() { this.sidebarOpen = false; },

        async init() {
            const session = await fetchSession();
            if (session) {
                this.isLoggedIn = true;
                this.username = session.username;
                this.applyRoute();
                this.ready = true;
            } else {
                // 未登录：直接跳登录页，不再停留展示「未登录」状态页（省一次点击）
                window.location.replace('/ui/login');
                return;
            }

            window.addEventListener('popstate', () => this.applyRoute());
        },

        applyRoute() {
            const { page, param } = resolve(window.location.pathname);
            Alpine.store('route').param = param;
            // 先置空再赋值，强制 x-if 重建页面组件，
            // 否则在同一页面内切换参数（如换一篇文章）不会重新加载数据
            this.currentPage = '';
            queueMicrotask(() => { this.currentPage = page; });
        },

        /** 侧边栏与页内链接统一走这里，避免整页刷新 */
        navigate(href) {
            if (window.location.pathname !== href) {
                window.history.pushState({}, '', href);
            }
            this.closeSidebar();
            this.applyRoute();
        },

        async logout() {
            try {
                await api.post('/auth/logout');
            } catch {
                // 登出失败也要清理前端状态，否则用户会卡在无法操作的界面
            }
            window.location.href = '/ui/login';
        },
    }));
});
