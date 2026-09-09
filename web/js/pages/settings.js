// 系统配置页面（L2 业务配置，后台可改且热生效）
//
// GET /api/v1/admin/settings 返回 { defaults, overrides }：defaults 是完整的
// L2 可配置项清单（结构即 DefaultL2Config），overrides 标出被后台显式改过的项。
// 本页把 defaults 按顶层分组拍平成 {key → value}，覆上 overrides，逐项保存/恢复。
//
// 分组顺序与中文标签在此固定，不依赖后端返回顺序（避免按 key 首字母排序的观感）。

// 分组显示顺序与中文名
const GROUP_ORDER = ['llm', 'embedding', 'reranker', 'search', 'chunk', 'keyword_search', 'mcp_log', 'collector', 'prompt'];
const GROUP_LABELS = {
    llm: 'LLM',
    embedding: 'Embedding',
    reranker: 'Reranker',
    search: '混合检索',
    chunk: '文本分块',
    keyword_search: '关键词检索',
    mcp_log: 'MCP 日志',
    collector: '网页采集',
    prompt: '提示词'
};

// 各配置项的说明文字（key → 说明）；未列出的显示默认文案
const FIELD_HINTS = {
    'llm.model': '模型名称，留空使用启动配置 QWEN_MODEL',
    'llm.temperature': '生成温度，0.0-2.0',
    'llm.max_tokens': '最大生成 token 数，100-8000',
    'llm.retry_count': '调用失败重试次数，0-5',
    'llm.timeout_ms': '单次调用超时（毫秒），5000-120000',
    'embedding.batch_size': 'Embedding 批量请求大小，1-64',
    'reranker.top_k': 'Reranker 候选数量，1-50',
    'search.top_k': '最终返回结果数，1-50 且 ≤ candidate_k',
    'search.candidate_k': 'RRF 召回候选数，5-200',
    'search.rrf_k': 'RRF 融合参数 k，1-200',
    'search.vector_weight': '向量检索权重，0.0-1.0（与 bm25_weight 之和为 1）',
    'search.bm25_weight': 'BM25 检索权重，0.0-1.0（与 vector_weight 之和为 1）',
    'search.ef_search': 'HNSW 查询时 ef_search，10-500',
    'chunk.size': 'Chunk 大小（字符数），200-800',
    'chunk.overlap': 'Chunk 重叠（字符数），0-200 且 < size',
    'keyword_search.tag_weight': '标签精确匹配权重，0.0-100.0',
    'keyword_search.category_weight': '分类精确匹配权重，0.0-100.0',
    'keyword_search.title_weight': '标题匹配权重，0.0-100.0',
    'keyword_search.summary_weight': '摘要匹配权重，0.0-100.0',
    'keyword_search.content_weight': '正文匹配权重，0.0-100.0',
    'mcp_log.retention_days': 'MCP 日志保留天数，1-365',
    'collector.user_agent': '抓网页时的 User-Agent，留空使用 config.yaml 的 COLLECTOR_USER_AGENT',
    'prompt.analyze': '文章分析提示词模板，支持 {{.Title}} / {{.Content}} / {{.Categories}}'
};

document.addEventListener('alpine:init', () => {
    Alpine.data('settings', () => ({
        csrf: '',
        groups: [],
        loading: true,
        saving: false,
        message: '',
        messageId: null,

        async init() {
            await Promise.all([this.loadCsrf(), this.loadSettings()]);
            this.loading = false;
        },

        // 写请求需 X-CSRF-Token，登录后随页面刷新丢失，故每次加载都重新获取
        async loadCsrf() {
            const res = await fetch('/api/v1/admin/auth/session');
            if (res.ok) {
                const data = await res.json();
                this.csrf = data.csrf_token || '';
            }
        },

        async loadSettings() {
            const res = await fetch('/api/v1/admin/settings');
            if (res.ok) {
                const data = await res.json();
                this.groups = this.flatten(data.defaults || {}, data.overrides || {});
            }
        },

        // 把嵌套 defaults 拍平成按 GROUP_ORDER 排序的分组字段，overrides 的值优先
        flatten(defaults, overrides) {
            const groupMap = {};
            const walk = (obj, prefix) => {
                for (const [k, v] of Object.entries(obj)) {
                    const key = prefix ? `${prefix}.${k}` : k;
                    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
                        walk(v, key);
                        continue;
                    }
                    const group = prefix || 'root';
                    if (!groupMap[group]) {
                        groupMap[group] = { key: group, label: GROUP_LABELS[group] || group, fields: [] };
                    }
                    const over = overrides[key];
                    const value = (over !== undefined && (typeof over === 'string' || typeof over === 'number'))
                        ? over
                        : v;
                    groupMap[group].fields.push({
                        key,
                        hint: FIELD_HINTS[key],
                        type: typeof v === 'number' ? 'number' : (typeof v === 'string' && v.length > 60 ? 'textarea' : 'text'),
                        value,
                        override: over !== undefined
                    });
                }
            };
            walk(defaults, '');

            const groups = Object.values(groupMap);
            groups.sort((a, b) => {
                const ia = GROUP_ORDER.indexOf(a.key);
                const ib = GROUP_ORDER.indexOf(b.key);
                return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
            });
            return groups;
        },

        async saveField(field) {
            this.saving = true;
            this.message = '';

            const raw = field.type === 'number' ? Number(field.value) : field.value;
            const res = await fetch(`/api/v1/admin/settings/${field.key}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf },
                body: JSON.stringify({ value: raw })
            });

            this.saving = false;
            if (res.ok) {
                this.messageId = 'success';
                this.message = `已保存 ${field.key}`;
                await this.loadSettings();
            } else {
                const data = await res.json();
                this.messageId = 'error';
                this.message = data.error || '保存失败';
            }
        },

        async resetField(field) {
            if (!confirm(`确定恢复 ${field.key} 到默认值？`)) return;

            const res = await fetch(`/api/v1/admin/settings/${field.key}/reset`, {
                method: 'POST',
                headers: { 'X-CSRF-Token': this.csrf }
            });

            if (res.ok) {
                this.messageId = 'success';
                this.message = `已恢复 ${field.key} 为默认值`;
                await this.loadSettings();
            }
        },

        async resetAll() {
            if (!confirm('确定恢复所有配置到默认值？')) return;

            const res = await fetch('/api/v1/admin/settings/reset', {
                method: 'POST',
                headers: { 'X-CSRF-Token': this.csrf }
            });

            if (res.ok) {
                this.messageId = 'success';
                this.message = '已恢复所有配置为默认值';
                await this.loadSettings();
            }
        }
    }));
});