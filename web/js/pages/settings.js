import { api } from '../api.js';

/**
 * 系统配置：展示默认值与库中覆盖值，支持修改与恢复默认。
 *
 * 数据形态：defaults 是按功能分组的嵌套对象（llm / search / chunk …），
 * overrides 则是扁平的点分键（llm.temperature → 0.3），
 * 两者由迁移 005 的播种方式决定。此处以 overrides 的扁平项为编辑单元，
 * 用 defaults 解析出每项的默认值以便对比。
 */
export function registerSettings(Alpine) {
    Alpine.data('settingsPage', () => ({
        loading: true,
        error: '',
        notice: '',

        defaults: {},
        overrides: {},

        editingKey: '',
        editingText: '',
        saving: false,

        historyKey: '',
        history: [],

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            try {
                const d = await api.get('/settings');
                this.defaults = d.defaults || {};
                this.overrides = d.overrides || {};
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        /** 按点分键的前缀分组，与 defaults 的顶层分组对应 */
        groups() {
            const byGroup = {};
            for (const [key, value] of Object.entries(this.overrides)) {
                const group = key.includes('.') ? key.split('.')[0] : 'other';
                (byGroup[group] ||= []).push({
                    key,
                    value,
                    defaultValue: this.defaultOf(key),
                });
            }
            return Object.keys(byGroup).sort().map((g) => ({
                name: g,
                items: byGroup[g].sort((a, b) => a.key.localeCompare(b.key)),
            }));
        },

        /** 顺着点分路径到 defaults 里取默认值 */
        defaultOf(key) {
            return key.split('.').reduce(
                (acc, part) => (acc && typeof acc === 'object' ? acc[part] : undefined),
                this.defaults,
            );
        },

        isModified(item) {
            return JSON.stringify(item.value) !== JSON.stringify(item.defaultValue);
        },

        fmt(v) {
            if (v === undefined) return '(无默认值)';
            return typeof v === 'object' ? JSON.stringify(v) : String(v);
        },

        edit(item) {
            this.editingKey = item.key;
            this.editingText = JSON.stringify(item.value);
            this.error = '';
            this.notice = '';
        },

        cancelEdit() {
            this.editingKey = '';
            this.editingText = '';
        },

        async save() {
            let parsed;
            try {
                parsed = JSON.parse(this.editingText);
            } catch (e) {
                this.error = 'JSON 格式有误：' + e.message + '（字符串需加引号，如 "abc"）';
                return;
            }

            this.saving = true;
            this.error = '';
            try {
                await api.put('/settings/' + encodeURIComponent(this.editingKey), parsed);
                this.notice = `已保存 ${this.editingKey}`;
                this.cancelEdit();
                await this.load();
            } catch (e) {
                this.error = e.message;
            } finally {
                this.saving = false;
            }
        },

        async reset(key) {
            if (!confirm(`确认将 ${key} 恢复为默认值？`)) return;
            try {
                await api.post('/settings/' + encodeURIComponent(key) + '/reset');
                this.notice = `${key} 已恢复默认`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async resetAll() {
            if (!confirm('确认恢复全部配置为默认值？所有自定义修改都会被清除。')) return;
            try {
                const r = await api.post('/settings/reset');
                this.notice = `已恢复 ${r.reset} 项配置`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async showHistory(key) {
            if (this.historyKey === key) {
                this.historyKey = '';
                this.history = [];
                return;
            }
            try {
                const d = await api.get('/settings/' + encodeURIComponent(key) + '/history');
                this.history = d.history || [];
                this.historyKey = key;
            } catch (e) {
                this.history = [];
                this.historyKey = key;
                if (e.status !== 404) this.error = e.message;
            }
        },
    }));
}
