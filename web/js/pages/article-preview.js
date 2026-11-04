import { api, formatTime, statusMeta, renderMarkdown } from '../api.js';

/**
 * 文章预览：以阅读形态展示 Markdown 正文 + 文章信息 + 网页截图。
 *
 * 与「文章编辑」页的分工：预览只读、面向阅读；编辑页负责改。列表标题点击
 * 进本页，操作列的「编辑」进编辑页。
 *
 * Markdown 渲染用 vendor 的 marked，输出经 DOMPurify 清洗——正文可能含从
 * 源站带回来的原始 HTML，直接注入会造成 XSS。
 */
export function registerArticlePreview(Alpine) {
    Alpine.data('articlePreviewPage', () => ({
        loading: true,
        error: '',

        article: null,
        renderedHTML: '',        // 清洗后的正文 HTML
        screenshotOpen: false,   // 截图灯箱

        get id() {
            return Alpine.store('route').param;
        },

        async init() {
            try {
                const a = await api.get('/articles/' + this.id);
                this.article = a;
                this.renderedHTML = renderMarkdown(a.content || '');
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        fmt: formatTime,
        meta: statusMeta,
    }));
}
