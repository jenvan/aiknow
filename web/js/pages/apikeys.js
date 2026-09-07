// API Key 管理页面
document.addEventListener('alpine:init', () => {
    Alpine.data('apiKeys', () => ({
        keys: [],
        loading: true,
        showCreate: false,
       createForm: {
            description: '',
            scope: 'readonly'
        },
        message: '',
        messageId: null,

        async init() {
            await this.loadKeys();
            this.loading = false;
        },

        async loadKeys() {
            const res = await fetch('/api/v1/admin/api-keys');
            if (res.ok) {
                this.keys = await res.json();
            }
        },

        async createKey() {
            this.showCreate = true;
            this.message = '';
        },

        async submitCreate(e) {
            e.preventDefault();
            this.saving = true;

            const res = await fetch('/api/v1/admin/api-keys', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this.createForm)
            });

            this.saving = false;

            if (res.ok) {
                const data = await res.json();
                alert(`创建成功！请保存好以下 Key（仅显示一次）：\n\n${data.key}\n\nKey 前缀：${data.key_prefix}`);
                this.showCreate = false;
                this.createForm = { description: '', scope: 'readonly' };
                await this.loadKeys();
            } else {
                const data = await res.json();
                this.messageId = 'error';
                this.message = data.error || '创建失败';
            }
        },

        async toggleKey(id, active) {
            const action = active ? 'enable' : 'disable';
            const res = await fetch(`/api/v1/admin/api-keys/${id}/${action}`, {
                method: 'POST'
            });

            if (res.ok) {
                await this.loadKeys();
            }
        },

        async deleteKey(id) {
            if (!confirm('确定删除此 API Key？')) return;

            const res = await fetch(`/api/v1/admin/api-keys/${id}`, {
                method: 'DELETE'
            });

            if (res.ok) {
                await this.loadKeys();
            }
        }
    }));
});
