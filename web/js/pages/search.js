// 检索测试页面
document.addEventListener('alpine:init', () => {
    Alpine.data('searchDebug', () => ({
        mode: 'vector',
        query: '',
        topK: 5,
        candidateK: 20,
        rrfK: 60,
        vectorWeight: 0.5,
        bm25Weight: 0.5,
        keywords: [],
        newKeyword: '',
        matchMode: 'any',
        loading: false,
        result: null,
        error: null,

        addKeyword() {
            const kw = this.newKeyword.trim();
            if (kw && !this.keywords.includes(kw)) {
                this.keywords.push(kw);
            }
            this.newKeyword = '';
        },

        removeKeyword(index) {
            this.keywords.splice(index, 1);
        },

        async search() {
            this.loading = true;
            this.result = null;
            this.error = null;

            try {
                let res;
                if (this.mode === 'vector') {
                    res = await fetch('/api/v1/search', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            query: this.query,
                            top_k: this.topK,
                            candidate_k: this.candidateK,
                            rrf_k: this.rrfK,
                            vector_weight: this.vectorWeight,
                            bm25_weight: this.bm25Weight
                        })
                    });
                } else {
                    res = await fetch('/api/v1/search/keywords', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            keywords: this.keywords,
                            match_mode: this.matchMode,
                            limit: this.topK
                        })
                    });
                }

                if (res.ok) {
                    this.result = await res.json();
                } else {
                    const data = await res.json();
                    this.error = data.error || '搜索失败';
                }
            } catch (err) {
                this.error = err.message;
            } finally {
                this.loading = false;
            }
        },

        async debugSearch() {
            this.loading = true;
            this.result = null;
            this.error = null;

            try {
                const res = await fetch('/api/v1/admin/search/debug', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        query: this.query,
                        params: {
                            top_k: this.topK,
                            candidate_k: this.candidateK,
                            rrf_k: this.rrfK,
                            vector_weight: this.vectorWeight,
                            bm25_weight: this.bm25Weight
                        }
                    })
                });

                if (res.ok) {
                    this.result = await res.json();
                } else {
                    const data = await res.json();
                    this.error = data.error || '调试失败';
                }
            } catch (err) {
                this.error = err.message;
            } finally {
                this.loading = false;
            }
        }
    }));
});
