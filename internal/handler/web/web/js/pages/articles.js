import { api, formatTime, statusMeta } from '../api.js';

/** 文章列表：筛选、搜索、排序、分页、行内与批量操作 */
export function registerArticles(Alpine) {
    Alpine.data('articlesPage', () => ({
        loading: true,
        error: '',
        notice: '',

        items: [],
        total: 0,
        pages: 0,

        // 查询条件
        page: 1,
        pageSize: 20,
        search: '',
        status: '',
        sortBy: 'created_at',
        sortOrder: 'desc',

        selected: new Set(),
        // Set 的变更不会被 Alpine 侦测到，用一个计数器强制视图刷新
        selectionTick: 0,

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            this.error = '';
            try {
                const q = new URLSearchParams({
                    page: this.page,
                    page_size: this.pageSize,
                    sort_by: this.sortBy,
                    sort_order: this.sortOrder,
                });
                if (this.search) q.set('search', this.search);
                if (this.status) q.set('status', this.status);

                const data = await api.get('/articles?' + q.toString());
                this.items = data.articles || [];
                this.total = data.total || 0;
                this.pages = data.pages || 0;
                this.clearSelection();
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        applyFilters() {
            this.page = 1;
            return this.load();
        },

        toggleSort(field) {
            if (this.sortBy === field) {
                this.sortOrder = this.sortOrder === 'desc' ? 'asc' : 'desc';
            } else {
                this.sortBy = field;
                this.sortOrder = 'desc';
            }
            return this.load();
        },

        goPage(p) {
            if (p < 1 || (this.pages && p > this.pages)) return;
            this.page = p;
            return this.load();
        },

        // ---- 选择 ----
        toggle(id) {
            if (this.selected.has(id)) this.selected.delete(id);
            else this.selected.add(id);
            this.selectionTick++;
        },
        isSelected(id) {
            this.selectionTick; // 建立依赖
            return this.selected.has(id);
        },
        toggleAll() {
            if (this.selected.size === this.items.length) this.selected.clear();
            else this.items.forEach((a) => this.selected.add(a.id));
            this.selectionTick++;
        },
        clearSelection() {
            this.selected.clear();
            this.selectionTick++;
        },
        get selectedCount() {
            this.selectionTick;
            return this.selected.size;
        },

        // ---- 操作 ----
        async remove(id, title) {
            if (!confirm(`确认删除《${title}》？该操作为软删除，可在数据库中恢复。`)) return;
            try {
                await api.del('/articles/' + id);
                this.notice = '已删除';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async reprocess(id) {
            try {
                await api.post(`/articles/${id}/reprocess`);
                this.notice = '已重新排队处理';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async batchDelete() {
            const ids = [...this.selected];
            if (!ids.length) return;
            if (!confirm(`确认删除选中的 ${ids.length} 篇文章？`)) return;
            try {
                const r = await api.post('/articles/batch-delete', ids);
                this.notice = `已删除 ${r.deleted} 篇`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async batchReprocess() {
            const ids = [...this.selected];
            if (!ids.length) return;
            try {
                const r = await api.post('/articles/batch-reprocess', ids);
                const failedCount = Object.keys(r.failed || {}).length;
                this.notice = failedCount
                    ? `成功 ${r.reprocessed} 篇，失败 ${failedCount} 篇`
                    : `已重新排队 ${r.reprocessed} 篇`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        fmt: formatTime,
        meta: statusMeta,
    }));
}
