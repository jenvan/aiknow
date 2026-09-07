// 系统配置页面
document.addEventListener('alpine:init', () => {
    Alpine.data('settings', () => ({
        settings: {},
        loading: true,
        saving: false,
        message: '',
        messageId: null,

        async init() {
            await this.loadSettings();
            this.loading = false;
        },

        async loadSettings() {
            const res = await fetch('/api/v1/admin/settings');
            if (res.ok) {
                this.settings = await res.json();
            }
        },

        async saveSetting(key) {
            this.saving = true;
            this.message = '';

            const value = this.settings[key];
            const res = await fetch(`/api/v1/admin/settings/${key}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value })
            });

            this.saving = false;

            if (res.ok) {
                this.messageId = 'success';
                this.message = '保存成功';
            } else {
                const data = await res.json();
                this.messageId = 'error';
                this.message = data.error || '保存失败';
            }
        },

        async resetSetting(key) {
            if (!confirm(`确定恢复 ${key} 到默认值？`)) return;

            const res = await fetch(`/api/v1/admin/settings/${key}/reset`, {
                method: 'POST'
            });

            if (res.ok) {
                await this.loadSettings();
            }
        },

        async resetAll() {
            if (!confirm('确定恢复所有配置到默认值？')) return;

            const res = await fetch('/api/v1/admin/settings/reset-all', {
                method: 'POST'
            });

            if (res.ok) {
                await this.loadSettings();
            }
        },

        async getHistory(key) {
            const res = await fetch(`/api/v1/admin/settings/${key}/history`);
            if (res.ok) {
                return await res.json();
            }
            return [];
        }
    }));
});
