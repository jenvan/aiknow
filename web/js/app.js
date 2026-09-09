function app() {
    return {
        currentPage: 'dashboard',
        isLoggedIn: false,

        async init() {
            // 检查登录状态
            try {
                const res = await fetch('/api/v1/admin/auth/session');
                if (res.ok) {
                    this.isLoggedIn = true;
                    this.updatePage();
                }
            } catch (e) {
                console.log('Not logged in');
            }

            // 监听路由变化
            window.addEventListener('popstate', () => this.updatePage());
        },

        updatePage() {
            const path = window.location.pathname;
            if (path.includes('/articles/new')) {
                this.currentPage = 'new-article';
            } else if (path.includes('/articles')) {
                this.currentPage = 'articles';
            } else if (path.includes('/search')) {
                this.currentPage = 'search';
            } else if (path.includes('/settings')) {
                this.currentPage = 'settings';
            } else {
                this.currentPage = 'dashboard';
            }
        },

        async logout() {
            try {
                await fetch('/api/v1/admin/auth/logout', { method: 'POST' });
            } catch (e) {}
            window.location.href = '/ui/login';
        }
    };
}
