import { api, renderMarkdown } from '../api.js';

/** 文章录入：手动录入 / URL 采集（含预览） */
export function registerArticleNew(Alpine) {
    Alpine.data('articleNewPage', () => ({
        mode: 'manual', // manual | url
        submitting: false,
        fetching: false,
        error: '',
        notice: '',
        createdId: '',

        form: { title: '', content: '', category_name: '', tags: '', url: '' },
        preview: null,

        /** 与「文章编辑」页同一套渲染（marked + DOMPurify），保证两侧观感一致 */
        get previewHTML() {
            return renderMarkdown(this.form.content);
        },

        /** 采集 URL 并预览，不落库 */
        async fetchUrl() {
            if (!this.form.url) {
                this.error = '请输入 URL';
                return;
            }
            this.fetching = true;
            this.error = '';
            this.preview = null;
            try {
                const r = await api.post('/articles/preview', { url: this.form.url });
                this.preview = r;
                // 采集结果填入表单，允许提交前手工修正。
                // 优先用 Markdown（保留标题/列表结构，分块更友好），无则退回纯文本。
                this.form.title = r.title || '';
                this.form.content = r.markdown || r.content || '';
            } catch (e) {
                this.error = e.message;
            } finally {
                this.fetching = false;
            }
        },

        async submit() {
            if (!this.form.title.trim() || !this.form.content.trim()) {
                this.error = '标题与正文不能为空';
                return;
            }

            this.submitting = true;
            this.error = '';
            this.notice = '';
            this.createdId = '';
            try {
                const tags = this.form.tags
                    .split(',')
                    .map((t) => t.trim())
                    .filter(Boolean);

                const body = {
                    title: this.form.title,
                    content: this.form.content,
                    source_type: this.mode === 'url' ? 'url' : 'manual',
                    tags,
                };
                if (this.form.category_name) body.category_name = this.form.category_name;
                if (this.mode === 'url' && this.form.url) body.source_url = this.form.url;

                const created = await api.post('/articles', body);
                this.createdId = created.id;
                this.notice = '录入成功，已排队进行 AI 处理';
                this.form = { title: '', content: '', category_name: '', tags: '', url: '' };
                this.preview = null;
            } catch (e) {
                // 409 表示内容重复，属正常业务结果，措辞上与故障区分开
                this.error = e.status === 409 ? `内容重复：${e.message}` : e.message;
            } finally {
                this.submitting = false;
            }
        },
    }));
}
