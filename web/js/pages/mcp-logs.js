// MCP 日志页面
document.addEventListener('alpine:init', () => {
    Alpine.data('mcpLogs', () => ({
        logs: [],
        total: 0,
        page: 1,
        pageSize: 20,
        filters: {
            trace_id: '',
            method: '',
            tool_name: '',
            keyword: '',
            dateFrom: '',
            dateTo: '',
            status: ''
        },
        selectedLog: null,
        loading: true,

        async init() {
            await this.loadLogs();
            this.loading = false;
        },

        async loadLogs() {
            const params = new URLSearchParams({
                page: this.page,
                page_size: this.pageSize,
                ...(this.filters.trace_id && { trace_id: this.filters.trace_id }),
                ...(this.filters.method && { method: this.filters.method }),
                ...(this.filters.tool_name && { tool_name: this.filters.tool_name }),
                ...(this.filters.keyword && { keyword: this.filters.keyword }),
                ...(this.filters.dateFrom && { date_from: this.filters.dateFrom }),
                ...(this.filters.dateTo && { date_to: this.filters.dateTo }),
                ...(this.filters.status && { status: this.filters.status })
            });

            const res = await fetch(`/api/v1/admin/mcp/logs?${params}`);
            if (res.ok) {
                const data = await res.json();
                this.logs = data.logs || [];
                this.total = data.total || 0;
            }
        },

        async viewLog(traceId) {
            const res = await fetch(`/api/v1/admin/mcp/logs/${traceId}`);
            if (res.ok) {
                this.selectedLog = await res.json();
            }
        },

        replayFromLog(log) {
            window.location.href = `/ui/mcp-debug?trace_id=${log.trace_id}`;
        },

        prevPage() {
            if (this.page > 1) {
                this.page--;
                this.loadLogs();
            }
        },

        nextPage() {
            if (this.page < Math.ceil(this.total / this.pageSize)) {
                this.page++;
                this.loadLogs();
            }
        },

        clearFilters() {
            this.filters = {
                trace_id: '',
                method: '',
                tool_name: '',
                keyword: '',
                dateFrom: '',
                dateTo: '',
                status: ''
            };
            this.page = 1;
            this.loadLogs();
        }
    }));
});
