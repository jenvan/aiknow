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
import { registerArticleNew } from './pages/article-new.js';
import { registerSearch } from './pages/search.js';
import { registerSettings } from './pages/settings.js';
import { registerApiKeys } from './pages/api-keys.js';
import { registerMcpDebug } from './pages/mcp-debug.js';
import { registerMcpLogs } from './pages/mcp-logs.js';

// 路由表：先匹配者优先，故 /articles/new 必须排在 /articles/:id 之前
const ROUTES = [
    { re: /^\/ui\/?$/,                    page: 'dashboard' },
    { re: /^\/ui\/dashboard\/?$/,         page: 'dashboard' },
    { re: /^\/ui\/articles\/new\/?$/,     page: 'article-new' },
    { re: /^\/ui\/articles\/([^/]+)\/?$/, page: 'article-edit' },
    { re: /^\/ui\/articles\/?$/,          page: 'articles' },
    { re: /^\/ui\/search\/?$/,            page: 'search' },
    { re: /^\/ui\/settings\/api-keys\/?$/, page: 'api-keys' },
    { re: /^\/ui\/settings\/?$/,          page: 'settings' },
    { re: /^\/ui\/mcp-debug\/?$/,         page: 'mcp-debug' },
    { re: /^\/ui\/mcp-logs\/?$/,          page: 'mcp-logs' },
];

const NAV_ITEMS = [
    { page: 'dashboard',   href: '/ui/dashboard',         label: '仪表盘' },
    { page: 'articles',    href: '/ui/articles',          label: '文章列表' },
    { page: 'article-new', href: '/ui/articles/new',      label: '录入文章' },
    { page: 'search',      href: '/ui/search',            label: '检索测试' },
    { page: 'mcp-debug',   href: '/ui/mcp-debug',         label: 'MCP 调试' },
    { page: 'mcp-logs',    href: '/ui/mcp-logs',          label: 'MCP 日志' },
    { page: 'settings',    href: '/ui/settings',          label: '系统配置' },
    { page: 'api-keys',    href: '/ui/settings/api-keys', label: 'API Key' },
];

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
    registerArticleNew(Alpine);
    registerSearch(Alpine);
    registerSettings(Alpine);
    registerApiKeys(Alpine);
    registerMcpDebug(Alpine);
    registerMcpLogs(Alpine);

    // 路由参数（如文章 ID）经全局 store 传给页面组件，
    // 避免把参数解析逻辑复制到每个页面
    Alpine.store('route', { param: '' });

    Alpine.data('shell', () => ({
        ready: false,
        isLoggedIn: false,
        username: '',
        currentPage: '',
        navItems: NAV_ITEMS,

        async init() {
            const session = await fetchSession();
            if (session) {
                this.isLoggedIn = true;
                this.username = session.username;
                this.applyRoute();
            }
            this.ready = true;

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
