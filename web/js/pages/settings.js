// 系统配置页面
//
// GET /api/v1/admin/settings 返回 { defaults, overrides }：defaults 是完整的
// L2 可配置项清单（结构即 DefaultL2Config），overrides 标出被后台显式改过的项。
// 本页把 defaults 按顶层分组拍平成 {key → value}，覆上 overrides，逐项保存/恢复。
document.addEventListener('alpine:init', () => {
    Alpine.data('settings', () => ({
        csrf: '',
        groups: [],
        loading: true,
        saving: false,
        message: '',
        messageId: null,

        async init() {
            await Promise.all([this.loadCsrf(), this.loadSettings()]);
            this.loading = false;
        },

        // 写请求需 X-CSRF-Token，登录后随页面刷新丢失，故每次加载都重新获取
        async loadCsrf() {
            const res = await fetch('/api/v1/admin/auth/session');
            if (res.ok) {
                const data = await res.json();
                this.csrf = data.csrf_token || '';
            }
        },

        async loadSettings() {
            const res = await fetch('/api/v1/admin/settings');
            if (res.ok) {
                const data = await res.json();
                this.groups = this.flatten(data.defaults || {}, data.overrides || {});
            }
        },

        // 把嵌套 defaults 拍平成分组字段，overrides 的值优先
        flatten(defaults, overrides) {
            const groupMap = {};
            const groups = [];
            const walk = (obj, prefix) => {
                for (const [k, v] of Object.entries(obj)) {
                    const key = prefix ? `${prefix}.${k}` : k;
                    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
                        walk(v, key);
                        continue;
                    }
                    const group = prefix || 'root';
                    if (!groupMap[group]) {
                        groupMap[group] = { label: group, fields: [] };
                        groups.push(groupMap[group]);
                    }
                    const over = overrides[key];
                    const value = (over !== undefined && (typeof over === 'string' || typeof over === 'number'))
                        ? over
                        : v;
                    groupMap[group].fields.push({
                        key,
                        type: typeof v === 'number' ? 'number' : (typeof v === 'string' && v.length > 60 ? 'textarea' : 'text'),
                        value,
                        override: over !== undefined
                    });
                }
            };
            walk(defaults, '');
            return groups;
        },

        async saveField(field) {
            this.saving = true;
            this.message = '';

            const raw = field.type === 'number' ? Number(field.value) : field.value;
            const res = await fetch(`/api/v1/admin/settings/${field.key}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf },
                body: JSON.stringify({ value: raw })
            });

            this.saving = false;
            if (res.ok) {
                this.messageId = 'success';
                this.message = `已保存 ${field.key}`;
                await this.loadSettings();
            } else {
                const data = await res.json();
                this.messageId = 'error';
                this.message = data.error || '保存失败';
            }
        },

        async resetField(field) {
            if (!confirm(`确定恢复 ${field.key} 到默认值？`)) return;

            const res = await fetch(`/api/v1/admin/settings/${field.key}/reset`, {
                method: 'POST',
                headers: { 'X-CSRF-Token': this.csrf }
            });

            if (res.ok) {
                this.messageId = 'success';
                this.message = `已恢复 ${field.key} 为默认值`;
                await this.loadSettings();
            }
        },

        async resetAll() {
            if (!confirm('确定恢复所有配置到默认值？')) return;

            const res = await fetch('/api/v1/admin/settings/reset', {
                method: 'POST',
                headers: { 'X-CSRF-Token': this.csrf }
            });

            if (res.ok) {
                this.messageId = 'success';
                this.message = '已恢复所有配置为默认值';
                await this.loadSettings();
            }
        }
    }));
});