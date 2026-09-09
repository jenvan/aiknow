import { api } from '../api.js';

/**
 * MCP 调试：参数表单、原始请求响应、鉴权 scope 切换。
 *
 * 走后端代理（POST /admin/mcp/debug）而非浏览器直连 /mcp，
 * 因此调试页无需持有 API Key。
 */
export function registerMcpDebug(Alpine) {
    Alpine.data('mcpDebugPage', () => ({
        running: false,
        error: '',

        method: 'tools/list',
        scope: 'readonly',
        tool: '',
        argsText: '{}',

        response: null,
        durationMs: 0,
        tools: [],

        async init() {
            await this.listTools();
        },

        async listTools() {
            try {
                const r = await api.post('/mcp/debug', { method: 'tools/list', scope: this.scope });
                this.tools = (r.response && r.response.tools) || [];
            } catch (e) {
                this.error = e.message;
            }
        },

        /** 选中工具时用其 schema 预填参数骨架，省去手写 */
        selectTool(name) {
            this.tool = name;
            this.method = 'tools/call';
            const t = this.tools.find((x) => x.name === name);
            const props = t && t.inputSchema && t.inputSchema.properties;
            if (props) {
                const skeleton = {};
                for (const k of Object.keys(props)) skeleton[k] = '';
                this.argsText = JSON.stringify(skeleton, null, 2);
            } else {
                this.argsText = '{}';
            }
        },

        async run() {
            this.error = '';
            this.response = null;

            let args = {};
            if (this.method === 'tools/call') {
                if (!this.tool) {
                    this.error = '请选择要调用的工具';
                    return;
                }
                try {
                    args = JSON.parse(this.argsText || '{}');
                } catch (e) {
                    this.error = '参数 JSON 格式有误：' + e.message;
                    return;
                }
            }

            this.running = true;
            try {
                const r = await api.post('/mcp/debug', {
                    method: this.method,
                    tool: this.tool,
                    arguments: args,
                    scope: this.scope,
                });
                this.durationMs = r.duration_ms || 0;
                // 后端对工具调用失败返回 200 + error 字段，便于展示详情
                this.response = r.error ? { error: r.error } : r.response;
                if (r.error) this.error = r.error;
            } catch (e) {
                this.error = e.message;
            } finally {
                this.running = false;
            }
        },

        async onScopeChange() {
            await this.listTools();
        },

        pretty(v) {
            try {
                return JSON.stringify(v, null, 2);
            } catch {
                return String(v);
            }
        },
    }));
}
