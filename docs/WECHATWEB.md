# 文件传输助手通道（网页版协议，只收不发）

> 一句话：手机上把公众号文章**转发给「文件传输助手」**，服务端收下链接入库，
> 回执发到微信 Bot。
>
> 实现：`pkg/wechatweb`（协议客户端）+ `internal/channel/wechatweb`（接收器），
> 内嵌在 `aiknow-server serve`，默认关闭（`WECHATWEB_ENABLED`）。
> **协议细节（登录/长轮询/消息 XML/错误码/抓包复现）见
> [WECHATWEB_PROTOCOL.md](WECHATWEB_PROTOCOL.md)。**

## 1. 为什么需要它

官方 iLink Bot（见 [WEIXIN_BOT.md](WEIXIN_BOT.md)）**收不到"转发"的卡片**——ClawBot
是插件会话，不在微信「转发给朋友」列表里。而「文件传输助手」是普通联系人，
转发是原生能力。两者互补：

| 通道 | 官方性 | 能收转发卡片 | 行为 |
|---|---|---|---|
| iLink Bot | ✅ 官方 | ❌ | 收发（粘贴链接 / 关键词检索） |
| 文件传输助手（本通道） | ❌ 逆向网页版协议 | ✅ | **只收不发** |

## 2. 支持哪些消息

| MsgType | 类型 | 处理 |
|---|---|---|
| `49`（AppMsgType=5） | 公众号文章卡片（转发） | ✅ 取 `appmsg.url` → 入库 |
| `1` | 文本 | ⚠️ **仅当文本里含 `mp.weixin.qq.com` 链接**时提取入库；纯文字忽略 |
| `1` | 粘贴的文章短链 | ✅ 同上（就是文本里的一条 URL） |
| `3` / `34` / 其它 | 图片 / 语音 / 文件 / 非公众号链接 | ❌ 忽略，仅 debug 日志 |

**不做检索**：这条通道没有问答能力（只收不发），查东西请用微信 Bot。
入库结果与"处理完成"都会通过 Bot 回执给你。

## 3. 后台扫码与状态管理

后台「微信入口」页（`/ui/weixin`）下半部分是「文件传输助手通道」：

| 功能 | 说明 |
|---|---|
| 状态 | 账号 / 同步游标项数 / 最近同步时间 / 接收器是否在跑 / 最近一次错误 |
| 启动·暂停 | 进程重启后（`WECHATWEB_ENABLED=false`）可手动启动/暂停接收，凭据保留 |
| 扫码登录 | 点「扫码登录」给出 `login.weixin.qq.com/qrcode/<uuid>` 链接，手机微信打开即扫码 |
| 登出 | 停止接收器并删除本地凭据 |

**扫码成功后接收器会当场启动，不需要重启 `serve`**（`Manager.Start`）。
`WECHATWEB_ENABLED` 只决定"启动时是否自动拉起"，不是扫码的前置条件。

## 4. 配置与命令

```bash
aiknow-server wechatweb login     # 扫码，凭据加密入库（只做一次 webwxinit）
aiknow-server wechatweb status    # 账号 / 同步游标项数 / 最近同步时间
aiknow-server wechatweb logout    # 删除本地凭据
```

`.env`：

| 变量 | 默认 | 说明 |
|---|---|---|
| `WECHATWEB_ENABLED` | `false` | 开启后 `serve` 启动接收器 |
| `SESSION_SECRET` | 必填 | 凭据加密密钥由它派生（SHA-256 → AES-256-GCM） |

## 5. 安全与风险（务必读完）

- **凭据加密落库**：`wechatweb_sessions.creds_enc` 是密文，明文只在内存；
  日志里不出现 skey / pass_ticket。
- **只收不发**：`webwxsendmsg` / `webwxsendmsgimg` / `webwxuploadmedia` **未实现**。
  协议里 `ToUserName` 是联系人 ID，一旦可写就等于能以账号身份给任意好友发消息。
- **独占**：同一账号两个消费者会抢消息、重复投递（实测）。开启本通道后
  **不要再开网页版文件传输助手**。
- **不自动重登**：会话失效（ret 1100/1101/1102）直接停止等人工扫码——
  反复重登是最容易触发风控的动作。
- **合规**：这是逆向网页版协议，不是官方接口，存在封号风险。
  **建议用不承载重要关系的小号**。

## 6. 实测到的协议坑（详见协议文档）

| 坑 | 说明 |
|---|---|
| `appid` | 是 `wx_webfilehelper`，不是通用网页版微信的 `wx782c26e4c19acffb` |
| 请求头 | **必须带 `mmweb_appid: wx_webfilehelper`**，漏了返回 `ret=1102` 且无错误信息 |
| 消息 `Content` | 两层 HTML 转义 + 真实 `<br/>`：先剥 br、解一层，再交给 XML 解析器处理 `&amp;` |
| `synccheck` | 页面用 `<script>` JSONP；服务端直接 GET 读文本 |
| 轮询节奏 | 空闲时挂 ~25 秒才返回，有消息才拉增量 —— 风控面最小 |

完整规则、样例与错误码见 [WECHATWEB_PROTOCOL.md](WECHATWEB_PROTOCOL.md)。

## 7. 排障

| 现象 | 排查 |
|---|---|
| 启动报「尚未登录」 | 先 `wechatweb login` |
| 启动报「凭据解密失败」 | `SESSION_SECRET` 变了；重新 login |
| 日志出现「会话已失效」 | 重新 login（不会自动重试） |
| 转发了没入库 | 看日志的 debug 是否「忽略非文章消息」——可能转发的是视频号/图片而不是公众号文章 |
| 收到回执但说「已经收录过」 | 去重命中（URL 归一后相同），正常 |
