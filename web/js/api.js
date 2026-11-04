/**
 * AiKnow 后台统一 API 客户端。
 *
 * 所有后台请求都应经由此处，原因有二：
 *   1. CSRF Token 必须附在每个写请求上，散落在各页面里迟早会漏；
 *   2. 401 需要统一跳登录，否则每个页面都要重复处理。
 */

const BASE = '/api/v1/admin';

// CSRF Token 保存在内存而非 localStorage：
// 它与服务端 Session 绑定，会话结束即失效，落盘只会增加泄漏面。
let csrfToken = '';

export function setCsrfToken(token) {
    csrfToken = token || '';
}

export function getCsrfToken() {
    return csrfToken;
}

/** 请求失败时抛出，携带 HTTP 状态码供调用方区分处理 */
export class ApiError extends Error {
    constructor(status, message) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
    }
}

async function request(method, path, body) {
    const opts = {
        method,
        headers: {},
        // 同源请求也显式带上 Cookie，避免未来改为跨域部署时静默失效
        credentials: 'same-origin',
    };

    if (body !== undefined) {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
    }

    // 安全方法无需 Token，与后端 CSRFMiddleware 的判定保持一致
    if (method !== 'GET' && method !== 'HEAD') {
        opts.headers['X-CSRF-Token'] = csrfToken;
    }

    const res = await fetch(BASE + path, opts);

    if (res.status === 401) {
        // 会话失效：跳回登录页，避免页面停在半可用状态
        window.location.href = '/ui/login';
        throw new ApiError(401, '登录态已失效');
    }

    // 204 及空响应体不做 JSON 解析
    let data = null;
    const text = await res.text();
    if (text) {
        try {
            data = JSON.parse(text);
        } catch {
            data = { error: text };
        }
    }

    if (!res.ok) {
        throw new ApiError(res.status, (data && data.error) || `请求失败（HTTP ${res.status}）`);
    }

    return data;
}

export const api = {
    get: (path) => request('GET', path),
    post: (path, body) => request('POST', path, body),
    put: (path, body) => request('PUT', path, body),
    del: (path) => request('DELETE', path),
};

/** 读取当前会话，同时刷新 CSRF Token。未登录返回 null 而不跳转。 */
export async function fetchSession() {
    const res = await fetch(BASE + '/auth/session', { credentials: 'same-origin' });
    if (!res.ok) return null;
    const data = await res.json();
    setCsrfToken(data.csrf_token);
    return data;
}

/** 时间格式化：后端统一返回 RFC3339 */
export function formatTime(value) {
    if (!value) return '-';
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return '-';
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
           `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** 处理状态的中文名与配色 */
export function statusMeta(status) {
    switch (status) {
        case 'completed':  return { label: '已完成', cls: 'bg-green-100 text-green-800' };
        case 'processing': return { label: '处理中', cls: 'bg-blue-100 text-blue-800' };
        case 'pending':    return { label: '待处理', cls: 'bg-gray-100 text-gray-700' };
        case 'failed':     return { label: '失败',   cls: 'bg-red-100 text-red-800' };
        default:           return { label: status || '-', cls: 'bg-gray-100 text-gray-700' };
    }
}

/**
 * Markdown → 清洗后的 HTML。复用 vendor 的 marked + DOMPurify。
 *
 * 正文可能含源站带回的原始 HTML，故必须经 DOMPurify 清洗后再注入，防 XSS。
 * marked/purify 未加载时降级为转义纯文本（<pre>），保证不白屏。
 */
export function renderMarkdown(md) {
    const text = md || '';
    const marked = window.marked;
    const purify = window.DOMPurify;
    if (!marked || !purify) {
        const div = document.createElement('div');
        div.textContent = text;
        return '<pre class="whitespace-pre-wrap">' + div.innerHTML + '</pre>';
    }
    const raw = marked.parse(text, { breaks: true, gfm: true });
    return purify.sanitize(raw, { USE_PROFILES: { html: true } });
}
