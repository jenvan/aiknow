// 文章列表页面
document.addEventListener('alpine:init', () => {
    Alpine.data('articleList', () => ({
        articles: [],
        total: 0,
        page: 1,
        pageSize: 20,
        filters: {
            category: '',
            tag: '',
            status: '',
            search: '',
            dateFrom: '',
            dateTo: ''
        },
        sort: { field: 'created_at', order: 'desc' },
        selectedIds: [],
        loading: true,

        async init() {
            await this.loadArticles();
            this.loading = false;
        },

        async loadArticles() {
            const params = new URLSearchParams({
                page: this.page,
                page_size: this.pageSize,
                sort_by: this.sort.field,
                sort_order: this.sort.order,
                ...(this.filters.search && { search: this.filters.search }),
                ...(this.filters.status && { status: this.filters.status })
            });

            const res = await fetch(`/api/v1/articles?${params}`);
            if (res.ok) {
                const data = await res.json();
                this.articles = data.articles || [];
                this.total = data.total || 0;
            }
        },

        selectAll(e) {
            this.selectedIds = e.target.checked ? this.articles.map(a => a.id) : [];
        },

        async batchDelete() {
            if (!confirm(`确定删除选中的 ${this.selectedIds.length} 篇文章？`)) return;

            const res = await fetch('/api/v1/admin/articles/batch-delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ids: this.selectedIds })
            });

            if (res.ok) {
                this.selectedIds = [];
                await this.loadArticles();
            }
        },

        async batchReprocess() {
            if (!confirm(`确定重新处理选中的 ${this.selectedIds.length} 篇文章？`)) return;

            const res = await fetch('/api/v1/admin/articles/batch-reprocess', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ids: this.selectedIds })
            });

            if (res.ok) {
                this.selectedIds = [];
                await this.loadArticles();
            }
        },

        async reprocess(id) {
            const res = await fetch(`/api/v1/admin/articles/${id}/reprocess`, { method: 'POST' });
            if (res.ok) await this.loadArticles();
        },

        async deleteArticle(id) {
            if (!confirm('确定删除这篇文章？')) return;

            const res = await fetch(`/api/v1/admin/articles/${id}`, { method: 'DELETE' });
            if (res.ok) await this.loadArticles();
        },

        async retryFailed(id) {
            await this.reprocess(id);
        },

        prevPage() {
            if (this.page > 1) {
                this.page--;
                this.loadArticles();
            }
        },

        nextPage() {
            if (this.page < Math.ceil(this.total / this.pageSize)) {
                this.page++;
                this.loadArticles();
            }
        }
    }));
});
