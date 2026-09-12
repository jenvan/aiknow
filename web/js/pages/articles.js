import { api, formatTime, statusMeta } from '../api.js';

/** 文章列表：BM25 全文检索 + 高级筛选（分类/标签/日期/排序） */
export function registerArticles(Alpine) {
    Alpine.data('articlesPage', () => ({
        loading: true,
        error: '',
        notice: '',

        items: [],
        total: 0,
        pages: 0,

        // ---- 基础筛选 ----
        searchQuery: '',   // BM25 关键词（传 search 参数）
        tokenized: true,   // 是否启用 jieba 分词（默认开启）
        status: '',

        // ---- 高级筛选（折叠） ----
        showAdvanced: false,
        categories: [],
        selectedCategory: '',
        allTags: [],          // 所有可选标签（从 API 加载）
        selectedTags: [],     // 已选标签 [{id, name}]
        tagSearch: '',        // 标签输入框文字（用于过滤下拉）
        tagDropdownOpen: false,
        dateFrom: '',
        dateTo: '',
        sortBy: 'created_at',
        sortOrder: 'desc',

        // ---- 分页 ----
        page: 1,
        pageSize: 20,

        // ---- 批量选择 ----
        selected: new Set(),
        selectionTick: 0,

        async init() {
            await this.loadMeta();
            await this.load();
        },

        async loadMeta() {
            try {
                const cats = await api.get('/categories');
                this.categories = cats || [];
            } catch (e) {
                console.error('加载分类失败', e);
            }
            try {
                const tags = await api.get('/tags');
                this.allTags = tags || [];
            } catch (e) {
                console.error('加载标签失败', e);
            }
        },

        // 标签下拉：过滤掉已选的，按输入文字模糊匹配
        get filteredTags() {
            const q = this.tagSearch.trim().toLowerCase();
            return this.allTags.filter(t =>
                !this.selectedTags.some(s => s.id === t.id) &&
                (!q || t.name.toLowerCase().includes(q))
            );
        },

        addTag(tag) {
            if (!this.selectedTags.some(t => t.id === tag.id)) {
                this.selectedTags.push(tag);
            }
            this.tagSearch = '';
            this.tagDropdownOpen = false;
            this.applyFilters();
        },

        removeTag(tag) {
            this.selectedTags = this.selectedTags.filter(t => t.id !== tag.id);
            this.applyFilters();
        },

        clearFilters() {
            this.searchQuery = '';
            this.tokenized = true;
            this.status = '';
            this.selectedCategory = '';
            this.selectedTags = [];
            this.tagSearch = '';
            this.dateFrom = '';
            this.dateTo = '';
            this.sortBy = 'created_at';
            this.sortOrder = 'desc';
            this.page = 1;
            return this.load();
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
                if (this.searchQuery.trim()) {
                    q.set('search', this.searchQuery.trim());
                    q.set('tokenized', this.tokenized ? '1' : '0');
                }
                if (this.status) q.set('status', this.status);
                if (this.selectedCategory) q.set('category_id', this.selectedCategory);
                if (this.selectedTags.length) q.set('tags', this.selectedTags.map(t => t.name).join(','));
                if (this.dateFrom) q.set('date_from', this.dateFrom);
                if (this.dateTo) q.set('date_to', this.dateTo);

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

        async applyFilters() {
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
            this.selectionTick;
            return this.selected.has(id);
        },
        toggleAll() {
            if (this.selected.size === this.items.length) this.selected.clear();
            else this.items.forEach(a => this.selected.add(a.id));
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
