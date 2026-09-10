import { api, formatTime } from '../api.js';

/**
 * API Key 管理：创建 / 启用 / 禁用 / 删除。
 *
 * 明文仅在创建响应中返回一次，服务端只存 SHA-256 哈希，
 * 故创建后必须显著提示用户立即保存。
 */
export function registerApiKeys(Alpine) {
    Alpine.data('apiKeysPage', () => ({
        loading: true,
        creating: false,
        error: '',
        notice: '',

        keys: [],
        form: { description: '', scope: 'readonly' },

        // 新建 Key 的明文，仅本次会话内可见
        newKey: null,
        copied: false,

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            try {
                const d = await api.get('/api-keys');
                this.keys = d.keys || [];
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        async create() {
            if (!this.form.description.trim()) {
                this.error = '请填写用途描述';
                return;
            }
            this.creating = true;
            this.error = '';
            this.notice = '';
            try {
                this.newKey = await api.post('/api-keys', {
                    description: this.form.description,
                    scope: this.form.scope,
                });
                this.copied = false;
                this.form.description = '';
                await this.load();
            } catch (e) {
                this.error = e.message;
            } finally {
                this.creating = false;
            }
        },

        async copyKey() {
            if (!this.newKey) return;
            try {
                await navigator.clipboard.writeText(this.newKey.key);
                this.copied = true;
            } catch {
                // 非 HTTPS 或权限被拒时剪贴板不可用，提示用户手工复制
                this.error = '复制失败，请手动选中并复制';
            }
        },

        dismissNewKey() {
            this.newKey = null;
            this.copied = false;
        },

        async toggle(key) {
            const action = key.is_active ? 'disable' : 'enable';
            try {
                await api.post(`/api-keys/${key.id}/${action}`);
                this.notice = key.is_active ? '已禁用' : '已启用';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async remove(key) {
            if (!confirm(`确认删除 Key「${key.description}」？使用该 Key 的客户端将立即失效。`)) return;
            try {
                await api.del('/api-keys/' + key.id);
                this.notice = '已删除';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        scopeLabel(s) {
            return s === 'readwrite' ? '读写' : '只读';
        },

        fmt: formatTime,
    }));
}
