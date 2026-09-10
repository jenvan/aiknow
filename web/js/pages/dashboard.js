import { api, formatTime, statusMeta } from '../api.js';

/**
 * 仪表盘：统计卡片、状态分布、队列深度、最近活动、服务健康。
 *
 * 四个接口各自独立容错——任一失败只让对应区块显示错误，
 * 不至于整页空白。
 */
export function registerDashboard(Alpine) {
    Alpine.data('dashboardPage', () => ({
        loading: true,
        stats: null,
        queue: null,
        health: null,
        recent: [],
        failed: [],
        errors: [],

        async init() {
            const settle = async (label, p) => {
                try {
                    return await p;
                } catch (e) {
                    this.errors.push(`${label}: ${e.message}`);
                    return null;
                }
            };

            const [stats, queue, health, activity] = await Promise.all([
                settle('统计', api.get('/dashboard/stats')),
                settle('队列', api.get('/dashboard/queue')),
                settle('健康检查', api.get('/dashboard/health')),
                settle('最近活动', api.get('/dashboard/activity')),
            ]);

            this.stats = stats;
            this.queue = queue;
            this.health = health;
            this.recent = (activity && activity.recent_articles) || [];
            this.failed = (activity && activity.failed_articles) || [];
            this.loading = false;
        },

        // 后端字段：total_articles / total_chunks / total_tags / total_categories
        cards() {
            const s = this.stats || {};
            return [
                { label: '文章总数', value: s.total_articles },
                { label: 'Chunk 总数', value: s.total_chunks },
                { label: '分类数', value: s.total_categories },
                { label: '标签数', value: s.total_tags },
            ];
        },

        // 后端字段：status_distribution
        statusList() {
            const dist = (this.stats && this.stats.status_distribution) || {};
            return ['completed', 'processing', 'pending', 'failed'].map((s) => ({
                key: s,
                count: dist[s] || 0,
                ...statusMeta(s),
            }));
        },

        queueAge() {
            if (!this.queue) return '-';
            const ms = this.queue.oldest_message_age_ms || 0;
            if (ms <= 0) return '队列为空';
            if (ms < 1000) return `${ms} 毫秒`;
            if (ms < 60000) return `${Math.round(ms / 1000)} 秒`;
            return `${Math.round(ms / 60000)} 分钟`;
        },

        healthRows() {
            const h = this.health || {};
            return [
                { label: '数据库', value: h.database },
                { label: 'LLM', value: h.llm_provider },
                { label: 'Embedding', value: h.embedding_provider },
                { label: 'Reranker', value: h.reranker_provider },
            ];
        },

        healthOK(v) {
            return v === 'connected' || (!!v && !String(v).startsWith('error'));
        },

        num(v) {
            return (v === null || v === undefined) ? '-' : v;
        },

        fmt: formatTime,
        meta: statusMeta,
    }));
}
