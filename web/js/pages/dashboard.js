// 仪表盘页面
document.addEventListener('alpine:init', () => {
    Alpine.data('dashboard', () => ({
        stats: {},
        queue: {},
        recentArticles: [],
        failedArticles: [],
        health: {},
        loading: true,

        async init() {
            await Promise.all([
                this.loadStats(),
                this.loadQueueStatus(),
                this.loadRecentActivity(),
                this.loadHealth()
            ]);
            this.loading = false;
        },

        async loadStats() {
            const res = await fetch('/api/v1/admin/dashboard/stats');
            if (res.ok) this.stats = await res.json();
        },

        async loadQueueStatus() {
            const res = await fetch('/api/v1/admin/dashboard/queue');
            if (res.ok) this.queue = await res.json();
        },

        async loadRecentActivity() {
            const res = await fetch('/api/v1/admin/dashboard/activity');
            if (res.ok) {
                const data = await res.json();
                this.recentArticles = data.recent_articles || [];
                this.failedArticles = data.failed_articles || [];
            }
        },

        async loadHealth() {
            const res = await fetch('/api/v1/admin/dashboard/health');
            if (res.ok) this.health = await res.json();
        }
    }));
});
