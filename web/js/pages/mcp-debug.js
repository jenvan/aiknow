// MCP 调试页面
document.addEventListener('alpine:init', () => {
    Alpine.data('mcpDebug', () => ({
        protocolVersion: '2025-06-18',
        tools: [],
        selectedTool: '',
        toolInput: {},
        inputMode: 'form',
        rawJson: '{}',
        response: null,
        responseHeaders: {},
        loading: false,
        error: null,
        useSession: true,
        apiKey: '',
        sseMode: false,
        history: [],
        maxHistory: 20,

        async init() {
            await this.listTools();
        },

        get currentTool() {
            return this.tools.find(t => t.name === this.selectedTool);
        },

        async listTools() {
            try {
                const res = await this.makeRequest({ method: 'tools/list' });
                if (res) {
                    this.tools = res.result || [];
                }
            } catch (err) {
                this.error = err.message;
            }
        },

        async initialize() {
            this.loading = true;
            this.error = null;
            try {
                const res = await this.makeRequest({
                    method: 'initialize',
                    params: {
                        protocolVersion: this.protocolVersion,
                        capabilities: {},
                        clientInfo: { name: 'aiknow-debug', version: '0.1.0' }
                    }
                });
                if (res) {
                    this.response = res;
                    this.addHistory('initialize', res);
                }
            } catch (err) {
                this.error = err.message;
            } finally {
                this.loading = false;
            }
        },

        async callTool() {
            if (!this.selectedTool) {
                this.error = '请选择工具';
                return;
            }

            this.loading = true;
            this.error = null;

            let arguments = {};
            if (this.inputMode === 'form') {
                arguments = this.toolInput;
            } else {
                try {
                    arguments = JSON.parse(this.rawJson);
                } catch (e) {
                    this.error = 'JSON 格式错误';
                    this.loading = false;
                    return;
                }
            }

            try {
                const res = await this.makeRequest({
                    method: 'tools/call',
                    params: {
                        name: this.selectedTool,
                        arguments
                    }
                });
                if (res) {
                    this.response = res;
                    this.addHistory(this.selectedTool, res);
                }
            } catch (err) {
                this.error = err.message;
            } finally {
                this.loading = false;
            }
        },

        async makeRequest(req) {
            const body = JSON.stringify({
                jsonrpc: '2.0',
                method: req.method,
                params: req.params || {},
                id: 1
            });

            const headers = {
                'Content-Type': 'application/json'
            };

            if (this.useSession) {
                // Session auth
            } else if (this.apiKey) {
                headers['Authorization'] = `Bearer ${this.apiKey}`;
            }

            const res = await fetch('/api/v1/admin/mcp/invoke', {
                method: 'POST',
                headers,
                body
            });

            if (!res.ok) {
                const data = await res.json().catch(() => ({}));
                throw new Error(data.error || `HTTP ${res.status}`);
            }

            return res.json();
        },

        addHistory(toolName, response) {
            this.history.unshift({
                tool: toolName,
                time: new Date().toLocaleTimeString(),
                response
            });
            if (this.history.length > this.maxHistory) {
                this.history.pop();
            }
        },

        replaysFromLog(traceId) {
            // TODO: 从日志重放
            this.error = '功能待实现';
        },

        switchInputMode(mode) {
            this.inputMode = mode;
            if (mode === 'json') {
                try {
                    this.rawJson = JSON.stringify(this.toolInput, null, 2);
                } catch (e) {
                    this.rawJson = '{}';
                }
            } else {
                try {
                    this.toolInput = JSON.parse(this.rawJson);
                } catch (e) {
                    // Keep form mode
                }
            }
        }
    }));
});
