import { api } from '../api.js';

/**
 * 分组展示顺序与中文名（与 CHANGELOG v0.6.2「按功能分组、不再按首字母排序」一致）。
 * 未在此列出的分组追加到末尾，保证新分组出现时不丢失。
 */
const GROUP_ORDER = ['prompt', 'llm', 'embedding', 'reranker', 'search', 'keyword_search', 'collector', 'chunk', 'mcp_log'];
const GROUP_LABELS = {
    llm: 'LLM',
    embedding: 'Embedding',
    reranker: 'Reranker',
    search: '混合检索',
    chunk: '文本分块',
    keyword_search: '关键词检索',
    mcp_log: 'MCP 日志',
    collector: '网页采集',
    prompt: '提示词',
};

// 各配置项的取值范围 / 含义说明（key → 说明）；未列出的不显示。
const FIELD_HINTS = {
    'llm.model': 'LLM 模型名，留空使用启动配置 QWEN_MODEL；优先级：调用方显式指定 > 本项 > L1 > 内置 qwen-plus',
    'llm.temperature': '生成温度 0-2，越高越发散',
    'llm.max_tokens': '单次生成最大 token，建议 100-8000',
    'llm.retry_count': '失败重试次数 0-5',
    'llm.timeout_ms': '单次调用超时（毫秒）',
    'embedding.batch_size': 'Embedding 批量请求大小（当前逐条调用，暂不消费）',
    'reranker.top_k': '送入精排的候选数，控制成本 1-50',
    'search.top_k': '最终返回结果数，需 ≤ candidate_k',
    'search.candidate_k': '向量+BM25 召回候选数 5-200',
    'search.rrf_k': 'RRF 融合参数 k，标准值 60',
    'search.vector_weight': '向量权重 0-1，与 bm25_weight 之和为 1',
    'search.bm25_weight': 'BM25 权重 0-1，与 vector_weight 之和为 1',
    'search.ef_search': 'HNSW 查询 ef_search 10-500',
    'chunk.strategy': '切分策略：structure 按空行+标题切段、超窗滑窗兜底（推荐）；window 纯定长滑窗（历史行为）',
    'chunk.size': '单片目标大小（字符），200-800；structure 下超过此值的段落才滑窗',
    'chunk.overlap': '滑窗重叠（字符），仅用于超长段落兜底，structure 边界处默认 0 不重叠，需 < size',
    'chunk.min_size': '短片段合并阈值：小于此值且并入上一片不超 size 时合并，防列表项切碎段',
    'keyword_search.tag_weight': '标签精确匹配权重',
    'keyword_search.category_weight': '分类精确匹配权重',
    'keyword_search.title_weight': '标题命中权重',
    'keyword_search.summary_weight': '摘要命中权重',
    'keyword_search.content_weight': '正文命中权重',
    'mcp_log.retention_days': 'MCP 日志保留天数 1-365',
    'collector.user_agent': '抓网页时的 User-Agent，留空用启动配置',
    'collector.max_content_length': '单次采集读取的 HTML 正文字节上限，留空/0 用启动默认（2MB）；页面被截断时可调大',
    'prompt.analyze': '文章分析提示词模板，支持 {{.Title}}/{{.Content}}/{{.Categories}}，须含 json 字样',
};

/**
 * 系统配置：展示默认值与库中覆盖值，支持修改与恢复默认。
 *
 * 数据形态：defaults 是按功能分组的嵌套对象（llm / search / chunk …），
 * overrides 则是扁平的点分键（llm.temperature → 0.3），
 * 两者由迁移 005 的播种方式决定。此处遍历完整的 defaults 展示全部可配项，
 * 每项的当前值取 overrides（若有）否则取默认，按固定分组顺序展示。
 */
export function registerSettings(Alpine) {
    Alpine.data('settingsPage', () => ({
        loading: true,
        error: '',
        notice: '',

        defaults: {},
        overrides: {},

        editingKey: '',
        editingText: '',
        editingIsString: false,
        saving: false,

        historyKey: '',
        history: [],

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            try {
                const d = await api.get('/settings');
                this.defaults = d.defaults || {};
                this.overrides = d.overrides || {};
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        /**
         * 把嵌套 defaults 拍平成「按功能分组」的列表：每个字段带当前值
         * （overrides 优先，否则默认值）、是否被改、取值范围说明。
         * 分组按 GROUP_ORDER 固定排序，未知分组追加到末尾。
         */
        groups() {
            const byGroup = {};

            // 遍历完整 defaults，让所有可配项都出现在页面（不只是被改过的）
            const walk = (obj, prefix) => {
                for (const [k, v] of Object.entries(obj)) {
                    const key = prefix ? `${prefix}.${k}` : k;
                    // 继续下钻嵌套对象（当前 L2 只有一层分组，但保持通用）
                    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
                        walk(v, key);
                        continue;
                    }
                    const group = prefix || 'other';
                    const override = this.overrides[key];
                    (byGroup[group] ||= []).push({
                        key,
                        // 有覆盖用覆盖值，否则用默认值
                        value: override !== undefined ? override : v,
                        defaultValue: v,
                        hint: FIELD_HINTS[key] || '',
                    });
                }
            };
            walk(this.defaults, '');

            // 组内按 key 稳定排序（组内顺序固定，不受影响）
            const allGroups = Object.keys(byGroup);
            const ordered = allGroups
                .map((g) => ({ name: g, items: byGroup[g].sort((a, b) => a.key.localeCompare(b.key)) }))
                .sort((a, b) => {
                    const ia = GROUP_ORDER.indexOf(a.name);
                    const ib = GROUP_ORDER.indexOf(b.name);
                    const ra = ia === -1 ? GROUP_ORDER.length : ia;
                    const rb = ib === -1 ? GROUP_ORDER.length : ib;
                    return ra - rb || a.name.localeCompare(b.name);
                });

            // 附上中文名与说明，供模板展示
            return ordered.map((g) => ({
                name: GROUP_LABELS[g.name] || g.name,
                key: g.name,
                items: g.items,
            }));
        },

        isModified(item) {
            return JSON.stringify(item.value) !== JSON.stringify(item.defaultValue);
        },

        fmt(v) {
            if (v === undefined) return '(无默认值)';
            return typeof v === 'object' ? JSON.stringify(v) : String(v);
        },

        edit(item) {
            this.editingKey = item.key;
            // 字符串型配置（llm.model / collector.user_agent / prompt.analyze 等）
            // 按裸文本编辑，避免把 JSON 引号暴露给用户；其余（数字/对象）仍走 JSON。
            this.editingIsString = typeof item.value === 'string';
            this.editingText = this.editingIsString ? item.value : JSON.stringify(item.value);
            this.error = '';
            this.notice = '';
        },

        cancelEdit() {
            this.editingKey = '';
            this.editingText = '';
            this.editingIsString = false;
        },

        async save() {
            let parsed;
            if (this.editingIsString) {
                // 字符串项：直接取输入文本（空串合法，表示"未覆盖/回落默认"）
                parsed = this.editingText;
            } else {
                try {
                    parsed = JSON.parse(this.editingText);
                } catch (e) {
                    this.error = 'JSON 格式有误：' + e.message;
                    return;
                }
            }

            this.saving = true;
            this.error = '';
            try {
                await api.put('/settings/' + encodeURIComponent(this.editingKey), { value: parsed });
                this.notice = `已保存 ${this.editingKey}`;
                this.cancelEdit();
                await this.load();
            } catch (e) {
                this.error = e.message;
            } finally {
                this.saving = false;
            }
        },

        async reset(key) {
            if (!confirm(`确认将 ${key} 恢复为默认值？`)) return;
            try {
                await api.post('/settings/' + encodeURIComponent(key) + '/reset');
                this.notice = `${key} 已恢复默认`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async resetAll() {
            if (!confirm('确认恢复全部配置为默认值？所有自定义修改都会被清除。')) return;
            try {
                const r = await api.post('/settings/reset');
                this.notice = `已恢复 ${r.reset} 项配置`;
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        async showHistory(key) {
            if (this.historyKey === key) {
                this.historyKey = '';
                this.history = [];
                return;
            }
            try {
                const d = await api.get('/settings/' + encodeURIComponent(key) + '/history');
                this.history = d.history || [];
                this.historyKey = key;
            } catch (e) {
                this.history = [];
                this.historyKey = key;
                if (e.status !== 404) this.error = e.message;
            }
        },
    }));
}
