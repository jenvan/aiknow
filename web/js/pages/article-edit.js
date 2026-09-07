// 文章编辑页面
document.addEventListener('alpine:init', () => {
    Alpine.data('articleEdit', () => ({
        article: {},
        chunks: [],
        categories: [],
        tags: [],
        loading: true,
        saving: false,
        message: '',
        messageId: null,

        async init(id) {
            await Promise.all([
                this.loadArticle(id),
                this.loadCategories(),
                this.loadTags()
            ]);
            this.loading = false;
        },

        async loadArticle(id) {
            const res = await fetch(`/api/v1/articles/${id}`);
            if (res.ok) this.article = await res.json();
        },

        async loadCategories() {
            const res = await fetch('/api/v1/admin/categories');
            if (res.ok) this.categories = await res.json();
        },

        async loadTags() {
            const res = await fetch('/api/v1/admin/tags');
            if (res.ok) this.tags = await res.json();
        },

        async loadChunks() {
            const res = await fetch(`/api/v1/admin/articles/${this.article.id}/chunks`);
            if (res.ok) {
                const data = await res.json();
                this.chunks = data.chunks || [];
            }
        },

        async save(e) {
            e.preventDefault();
            this.saving = true;
            this.message = '';

            const reprocess = e.target.dataset.reprocess === 'true';

            const res = await fetch(`/api/v1/articles/${this.article.id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    ...this.article,
                    category_id: this.article.category_id,
                    tags: this.article.tags?.map(t => t.name) || []
                })
            });

            this.saving = false;

            if (res.ok) {
                this.messageId = 'success';
                this.message = '保存成功';
                await this.loadArticle(this.article.id);
                if (reprocess) {
                    await this.reprocess();
                }
            } else {
                const data = await res.json();
                this.messageId = 'error';
                this.message = data.error || '保存失败';
            }
        },

        async reprocess() {
            const res = await fetch(`/api/v1/admin/articles/${this.article.id}/reprocess`, {
                method: 'POST'
            });
            if (res.ok) {
                await this.loadArticle(this.article.id);
            }
        },

        async deleteArticle() {
            if (!confirm('确定删除这篇文章？')) return;

            const res = await fetch(`/api/v1/admin/articles/${this.article.id}`, {
                method: 'DELETE'
            });

            if (res.ok) {
                window.location.href = '/ui/articles';
            }
        },

        addTag() {
            if (!this.article.tags) this.article.tags = [];
            this.article.tags.push({ name: '' });
        },

        removeTag(index) {
            this.article.tags.splice(index, 1);
        }
    }));
});
