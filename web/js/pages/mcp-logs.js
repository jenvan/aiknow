import { api, formatTime } from '../api.js';

/** MCP 请求日志：筛选、全文检索、trace_id 查询、查看详情 */
export function registerMcpLogs(Alpine) {
    Alpine.data('mcpLogsPage', () => ({
        loading: true,
        error: '',
        notice: '',

        logs: [],
        page: 1,
        pageSize: 20,

        filters: { trace_id: '', tool_name: '', status: '', keyword: '' },
        detail: null,

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            this.error = '';
            try {
                const q = new URLSearchParams({ page: this.page, page_size: this.pageSize });
                for (const [k, v] of Object.entries(this.filters)) {
                    if (v) q.set(k, v);
                }
                const d = await api.get('/logs?' + q.toString());
                this.logs = d.logs || [];
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        applyFilters() {
            this.page = 1;
            return this.load();
        },

        resetFilters() {
            this.filters = { trace_id: '', tool_name: '', status: '', keyword: '' };
            this.page = 1;
            return this.load();
        },

        goPage(p) {
            if (p < 1) return;
            this.page = p;
            return this.load();
        },

        async showDetail(traceId) {
            if (this.detail && this.detail.trace_id === traceId) {
                this.detail = null;
                return;
            }
            try {
                this.detail = await api.get('/logs/' + encodeURIComponent(traceId));
            } catch (e) {
                this.error = e.message;
            }
        },

        async purge() {
            const days = prompt('清理多少天前的日志？', '30');
            if (days === null) return;
            const n = parseInt(days, 10);
            if (Number.isNaN(n) || n < 0) {
                this.error = '请输入非负整数';
                return;
            }
            try {
                const r = await api.del('/logs?days=' + n);
                this.notice = `已清理 ${r.deleted} 条日志`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        statusCls(s) {
            return s === 'error'
                ? 'bg-red-100 text-red-800'
                : 'bg-green-100 text-green-800';
        },

        pretty(v) {
            if (v === null || v === undefined) return '-';
            try {
                return JSON.stringify(typeof v === 'string' ? JSON.parse(v) : v, null, 2);
            } catch {
                return String(v);
            }
        },

        fmt: formatTime,
    }));
}
