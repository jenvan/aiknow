import { api } from '../api.js';

/** 检索测试：混合检索分阶段结果、耗时明细、关键词检索对照 */
export function registerSearch(Alpine) {
    Alpine.data('searchPage', () => ({
        mode: 'hybrid', // hybrid | keywords
        running: false,
        error: '',

        query: '',
        keywords: '',
        params: { top_k: 10, candidate_k: 20, rrf_k: 60, vector_weight: 1, bm25_weight: 1, ef_search: 40, category: '' },
        kparams: { top_k: 10, tag_weight: 3, category_weight: 2.5, title_weight: 2, summary_weight: 1.5, content_weight: 1 },

        result: null,
        keywordResult: null,

        async run() {
            this.error = '';
            this.running = true;
            this.result = null;
            this.keywordResult = null;
            try {
                if (this.mode === 'hybrid') {
                    if (!this.query.trim()) throw new Error('请输入检索词');
                    const body = { query: this.query, params: this.params };
                    if (body.params.category === '') delete body.params.category;
                    this.result = await api.post('/search/debug', body);
                } else {
                    const kws = this.keywords.split(',').map((k) => k.trim()).filter(Boolean);
                    if (!kws.length) throw new Error('请输入至少一个关键词');
                    this.keywordResult = await api.post('/search/debug/keywords', {
                        keywords: kws,
                        params: this.kparams,
                    });
                }
            } catch (e) {
                this.error = e.message;
            } finally {
                this.running = false;
            }
        },

        /** 各阶段结果：DebugSearch 返回 vector_result / bm25_result / rerank_result */
        stages() {
            if (!this.result) return [];
            return [
                { key: 'vector', label: '向量检索', data: this.result.vector_result },
                { key: 'bm25', label: 'BM25 检索', data: this.result.bm25_result },
                { key: 'rerank', label: '精排结果', data: this.result.rerank_result },
            ].filter((s) => s.data);
        },

        timings() {
            const t = (this.result && this.result.rerank_result && this.result.rerank_result.timings)
                || (this.result && this.result.vector_result && this.result.vector_result.timings);
            if (!t) return [];
            return [
                { label: 'Embedding', ms: t.embedding_ms },
                { label: '向量检索', ms: t.vector_ms },
                { label: 'BM25', ms: t.bm25_ms },
                { label: 'RRF 融合', ms: t.rrf_ms },
                { label: '精排', ms: t.rerank_ms },
                { label: '总计', ms: t.total_ms },
            ];
        },

        score(v) {
            return typeof v === 'number' ? v.toFixed(4) : '-';
        },
    }));
}
