import { api, formatTime } from '../api.js';

/**
 * 微信入口页：两条通道
 *
 *  1. iLink Bot（官方）：收链接/关键词，能收发；扫码后立即生效。
 *  2. 文件传输助手（网页版协议）：只收转发来的公众号文章，零发送；
 *     扫码成功后后端会**当场拉起接收器**，不需要重启服务。
 *
 * 扫码都不渲染二维码图片：后端给出 login.weixin.qq.com 的链接，
 * 手机微信打开就等于扫码，前端保持零依赖。
 */
export function registerWeixin(Alpine) {
    Alpine.data('weixinPage', () => ({
        loading: true,
        error: '',
        notice: '',

        status: null,
        loginUrl: '',
        loginStatus: '',
        pollTimer: null,
        copied: false,

        web: null,
        loadingWeb: true,
        webLoginUrl: '',
        webLoginStatus: '',
        webPollTimer: null,
        webCopied: false,

        async init() {
            await Promise.all([this.load(), this.loadWeb()]);
        },

        // ---------- iLink Bot ----------
        async load() {
            this.loading = true;
            try {
                this.status = await api.get('/weixin/status');
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
                this.loginQr = d.qr || '';
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
                        this.loginQr = '';
                        this.loginStatus = '';
                        this.notice = `扫码成功，已保存 bot ${d.bot_id || ''}`;
                        await this.load();
                        return;
                    }
                    if (d.status === 'expired' || d.status === 'none') {
                        this.loginUrl = '';
                        this.loginQr = '';
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

        async logout() {
            if (!confirm('确认删除 Bot 凭据？重新启用需要再扫一次码。')) return;
            this.error = '';
            this.notice = '';
            try {
                await api.post('/weixin/logout', {});
                this.notice = '已删除 Bot 凭据';
                await this.load();
            } catch (e) {
                this.error = e.message;
            }
        },

        // ---------- 文件传输助手 ----------
        async loadWeb() {
            this.loadingWeb = true;
            try {
                this.web = await api.get('/wechatweb/status');
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loadingWeb = false;
            }
        },

        async startWebLogin() {
            this.error = '';
            this.notice = '';
            this.webCopied = false;
            try {
                const d = await api.post('/wechatweb/login/start', {});
                this.webLoginUrl = d.url;
                // 官方图（login.weixin.qq.com/qrcode/...）优先：扫码内容由微信自己保证
                this.webLoginQr = d.img || d.qr || '';
                this.webLoginStatus = 'wait';
                this.pollWebLogin();
            } catch (e) {
                this.error = e.message;
            }
        },

        pollWebLogin() {
            clearTimeout(this.webPollTimer);
            const tick = async () => {
                try {
                    const d = await api.get('/wechatweb/login/poll');
                    this.webLoginStatus = d.status;
                    if (d.status === 'confirmed') {
                        this.webLoginUrl = '';
                        this.webLoginQr = '';
                        this.webLoginStatus = '';
                        this.notice = `文件传输助手已登录（${d.user_name || ''}），接收器已启动`;
                        await this.loadWeb();
                        return;
                    }
                    if (d.status === 'none') {
                        this.webLoginUrl = '';
                        this.webLoginQr = '';
                        this.webLoginStatus = '';
                        return;
                    }
                    this.webPollTimer = setTimeout(tick, 2000);
                } catch (e) {
                    this.error = e.message;
                }
            };
            this.webPollTimer = setTimeout(tick, 1500);
        },

        async startWeb() {
            this.error = '';
            this.notice = '';
            try {
                await api.post('/wechatweb/start', {});
                this.notice = '接收器已启动';
                await this.loadWeb();
            } catch (e) {
                this.error = e.message;
            }
        },

        async stopWeb() {
            this.error = '';
            this.notice = '';
            try {
                await api.post('/wechatweb/stop', {});
                this.notice = '接收器已停止（凭据保留）';
                await this.loadWeb();
            } catch (e) {
                this.error = e.message;
            }
        },

        async copyWebUrl() {
            try {
                await navigator.clipboard.writeText(this.webLoginUrl);
                this.webCopied = true;
            } catch {
                this.error = '复制失败，请手动选中链接';
            }
        },

        async logoutWeb() {
            if (!confirm('确认删除文件传输助手凭据？接收器会停止，重新启用需再扫一次码。')) return;
            this.error = '';
            this.notice = '';
            try {
                await api.post('/wechatweb/logout', {});
                this.notice = '已删除文件传输助手凭据';
                await this.loadWeb();
            } catch (e) {
                this.error = e.message;
            }
        },

        statusLabel(s) {
            if (!s) return '';
            if (s === 'wait') return '等待扫码…';
            if (s === 'scaned') return '已扫码，请在手机上确认';
            return s;
        },

        fmt: formatTime,
    }));
}
