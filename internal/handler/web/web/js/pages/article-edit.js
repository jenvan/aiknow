import { api, formatTime, statusMeta } from '../api.js';

/**
 * 文章编辑：正文编辑、AI 结果修正、状态面板、Chunk 预览。
 *
 * 关键行为约定（AGENT.md）：改正文保存会把 processing_status 重置为 pending
 * 并重新入队；仅改 AI 结果（摘要/分类/标签）不触发重新处理。
 * 该判定在前端提示、由后端 Update + Reprocess 组合实现。
 */
export function registerArticleEdit(Alpine) {
    Alpine.data('articleEditPage', () => ({
        loading: true,
        saving: false,
        error: '',
        notice: '',

        article: null,
        chunks: [],
        chunksLoaded: false,

        // 表单字段
        form: { title: '', content: '', category_name: '', tags: '', summary: '' },
        originalContent: '',

        get id() {
            return Alpine.store('route').param;
        },

        get contentChanged() {
            return this.form.content !== this.originalContent;
        },

        async init() {
            try {
                const a = await api.get('/articles/' + this.id);
                this.article = a;
                this.originalContent = a.content || '';
                this.form = {
                    title: a.title || '',
                    content: a.content || '',
                    category_name: a.category_name || '',
                    tags: (a.tags || []).map((t) => t.name).join(', '),
                    summary: a.summary || '',
                };
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        async loadChunks() {
            if (this.chunksLoaded) return;
            try {
                const d = await api.get(`/articles/${this.id}/chunks`);
                this.chunks = d.chunks || [];
                this.chunksLoaded = true;
            } catch (e) {
                this.error = e.message;
            }
        },

        async save() {
            this.saving = true;
            this.error = '';
            this.notice = '';
            try {
                const tags = this.form.tags
                    .split(',')
                    .map((t) => t.trim())
                    .filter(Boolean);

                const contentChanged = this.contentChanged;

                await api.put('/articles/' + this.id, {
                    title: this.form.title,
                    content: this.form.content,
                    category_name: this.form.category_name || undefined,
                    tags,
                });

                // 正文变更需重新走 AI 流水线，否则摘要与向量会与正文脱节
                if (contentChanged) {
                    await api.post(`/articles/${this.id}/reprocess`);
                    this.notice = '已保存，正文有变更，已重新排队处理';
                    this.chunksLoaded = false;
                    this.chunks = [];
                } else {
                    this.notice = '已保存（未改正文，不触发重新处理）';
                }

                this.originalContent = this.form.content;
                const a = await api.get('/articles/' + this.id);
                this.article = a;
            } catch (e) {
                this.error = e.message;
            } finally {
                this.saving = false;
            }
        },

        async reprocess() {
            try {
                await api.post(`/articles/${this.id}/reprocess`);
                this.notice = '已重新排队处理';
                this.article = await api.get('/articles/' + this.id);
            } catch (e) {
                this.error = e.message;
            }
        },

        fmt: formatTime,
        meta: statusMeta,
    }));
}
