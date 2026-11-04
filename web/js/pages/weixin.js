import { api, formatTime } from '../api.js';

/**
 * 微信 Bot 页面：状态、重新扫码、白名单、登出。
 *
 * 扫码不渲染二维码图片，而是给出 liteapp.weixin.qq.com 的链接——
 * 那链接用手机微信打开就等于扫码，页面只负责展示/复制 + 轮询结果，
 * 这样零构建的前端不必引入 qrcode 库。
 */
export function registerWeixin(Alpine) {
    Alpine.data('weixinPage', () => ({
        loading: true,
        error: '',
        notice: '',

        status: null,
        allowFromDraft: '',
        saving: false,

        loginUrl: '',
        loginStatus: '',
        pollTimer: null,
        copied: false,

        async init() {
            await this.load();
        },

        async load() {
            this.loading = true;
            try {
                this.status = await api.get('/weixin/status');
                this.allowFromDraft = this.status.allow_from || '';
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },

        async startLogin() {
            this.error = '';
            this.notice = '';
            this.copied = false;
            try {
                const d = await api.post('/weixin/login/start', {});
                this.loginUrl = d.url;
                this.loginStatus = 'wait';
                this.pollLogin();
            } catch (e) {
                this.error = e.message;
            }
        },

        pollLogin() {
            clearTimeout(this.pollTimer);
            const tick = async () => {
                try {
                    const d = await api.get('/weixin/login/poll');
                    this.loginStatus = d.status;
                    if (d.status === 'confirmed') {
                        this.loginUrl = '';
                        this.loginStatus = '';
                        this.notice = `扫码成功，已保存 bot ${d.bot_id || ''}`;
                        await this.load();
                        return;
                    }
                    if (d.status === 'expired' || d.status === 'none') {
                        this.loginUrl = '';
                        this.loginStatus = '';
                        if (d.status === 'expired') this.error = '二维码已过期，请重新扫码';
                        return;
                    }
                    this.pollTimer = setTimeout(tick, 2000);
                } catch (e) {
                    this.error = e.message;
                }
            };
            this.pollTimer = setTimeout(tick, 1500);
        },

        async copyUrl() {
            try {
                await navigator.clipboard.writeText(this.loginUrl);
                this.copied = true;
            } catch {
                this.error = '复制失败，请手动选中链接';
            }
        },

        async saveAllowFrom() {
            this.saving = true;
            this.error = '';
            this.notice = '';
            try {
                const d = await api.put('/weixin/allow-from', { allow_from: this.allowFromDraft });
                this.allowFromDraft = d.allow_from || '';
                this.notice = '白名单已保存';
                await this.load();
            } catch (e) {
                this.error = e.message;
            } finally {
                this.saving = false;
            }
        },

        async logout() {
            if (!confirm('确认删除本地保存的 bot 凭据？重新启用需要再扫一次码。')) return;
            this.error = '';
            this.notice = '';
            try {
                await api.post('/weixin/logout', {});
                this.notice = '已删除本地凭据';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        statusLabel(s) {
            if (!s) return '未登录';
            if (s === 'wait') return '等待扫码…';
            if (s === 'scaned') return '已扫码，等待手机确认…';
            return s;
        },

        fmt: formatTime,
    }));
}
