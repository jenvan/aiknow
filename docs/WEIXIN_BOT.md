# 微信 Bot 入口（iLink / ClawBot）

> 一句话：在微信里把公众号文章链接发给 AiKnow 就直接入库，发关键词就搜知识库。
>
> 实现：`pkg/ilink`（协议客户端）+ `internal/channel/weixin`（意图路由），
> **内嵌在 `aiknow-server serve` 进程内**，默认关闭（`WEIXIN_BOT_ENABLED`）。

## 1. 为什么是 iLink

微信 2026 年通过 OpenClaw 开放了个人号官方 Bot 接口，协议名 **iLink**，
域名 `ilinkai.weixin.qq.com`，纯 HTTP/JSON，无逆向、无封号风险：

| 能力 | 端点 | 用法 |
|------|------|------|
| 扫码登录 | `GET /ilink/bot/get_bot_qrcode`、`GET /ilink/bot/get_qrcode_status` | 一次性 |
| 收消息 | `POST /ilink/bot/getupdates` | 长轮询，服务端 hold 35s |
| 发消息 | `POST /ilink/bot/sendmessage` | 必须回带入站消息的 `context_token` |

**不需要 OpenClaw 账号、不需要审核**：实测 `get_bot_qrcode` 无鉴权即可返回二维码。

前提：手机微信 8.0.70+，`我 → 设置 → 插件 → 微信 ClawBot`。

## 2. 交互约定（v1：只支持单聊）

| 用户输入 | 行为 | 回包 |
|---|---|---|
| 含 `http(s)://` 的文本 | **秒回执** → 后台采集 → 入库 → 入队 | ①「⏳ 正在采集」②「✅ 已收录」+ 标题/原文链接（异步补发） |
| 纯文本（≤8 字） | 关键词检索（标签/分类加权） | Top5 文章卡片（标题/分类/标签/原文链接） |
| 纯文本（>8 字且关键词零命中） | 降级混合语义检索 | Top3 片段 + 文章标题/原文链接 |
| `/help` `/status` | 帮助 / 最近 5 篇入库状态 | 文本 |

**关键取舍：先关键词、后语义**。关键词检索几十毫秒、零 embedding 成本，且
「打了这个标签的文章」正是用户预期；只有输入够长且关键词零命中时才走
embedding + rerank，避免每条消息都付费。

**为什么是「粘贴链接」而不是「转发卡片」**：实测微信「转发给朋友」列表里
**没有 ClawBot 会话**（它是插件会话，不是好友），公众号文章卡片无法转发进来。
所以入口是：文章页右上角 `···` → 复制链接 → 粘贴发送。iLink 的
`item_list[].type` 也只定义了 1-5（文本/图片/语音/文件/视频），没有卡片类型。
`pkg/ilink` 用 `Item.Raw` 保留未知类型的原始 JSON，未来微信若加卡片类型不必改协议层。

## 3. 架构

```
微信用户 ──长轮询──▶ internal/channel/weixin (bot.go)
                          │  白名单 → message_id 去重 → 意图路由
                          ▼
                     router.go ──┬─ URL ──▶ Collector.Fetch → ArticleService.Create（去重+入队）
                                 ├─ 关键词 ─▶ KeywordSearchService
                                 └─ 长问句 ─▶ SearchService（embedding + rerank）
                          │
                          ▼
                    client.SendText（带 context_token）
```

channel 层**不重复实现检索/采集**：它只做「消息 → 意图 → 现有 service 调用」，
与 MCP 工具、REST API 共用同一套语义（对齐 `add_article_by_url` /
`find_articles_by_keywords` / `search_knowledge`）。

## 4. 数据：`weixin_bot_accounts`（迁移 012）

| 列 | 说明 |
|---|---|
| `bot_id` / `bot_token` / `base_url` | 扫码得到的凭据（L3，**不进 `.env`**、不打印） |
| `owner_user_id` | 扫码者，默认唯一被允许的用户 |
| `allow_from` | 逗号分隔白名单 |
| `get_updates_cursor` | `get_updates_buf` 消费位点 |
| `last_context_token` | 最近一次入站的 `context_token`（供将来主动补发） |

两张表都只有一行（v1 单 bot，取 `updated_at` 最新的一行）。

## 4.1 采集是异步的（发完就走）

链接消息的处理分两步，用户不用盯着：

1. **立刻回执**「⏳ 正在采集」（1 秒内）；
2. 采集任务丢给 `Runner` 后台执行（并发上限 2），完成后补发「✅ 已收录」。
   采集失败/重复则补发对应提示。

两个配套优化：

- **快速采集**：录入走 `Collector.FetchLight`——只抓 http、不渲染不截图（公众号
  1-3 秒；渲染要约 30 秒）。正文过短（典型 SPA 空壳）才回退渲染。截图的活交给
  worker 处理时做，所以这里不重复渲染。
- **长轮询不再被阻塞**：此前同步采集会让后续消息全部排队，现在 handler 立即返回。

## 4.2 处理完成主动推送

采集入库时不回复「处理完了」—— AI 处理是异步的。真正完成时由 worker 回调
`Pusher.Deliver`，用**入站消息的 context_token** 主动发一条：

```
✅ 处理完成《标题》
摘要：…
分类 技术 · 标签 golang/并发
原文：https://…
```

只推由微信入口发起的采集（`Pusher` 用 articleID 登记表区分来源），后台/API 录入
的文章不会推给微信；登记超过 30 分钟未兑现即丢弃。

## 5. 配置与命令

L1 配置（`.env`）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `WEIXIN_BOT_ENABLED` | `false` | 开启后 `serve` 启动长轮询 |
| `WEIXIN_BOT_ALLOW_FROM` | 空 | 非空时覆盖库中白名单；空 = 仅扫码者本人 |
| `WEIXIN_BOT_BASE_URL` | 空 | 空则用 `https://ilinkai.weixin.qq.com` |

后台页面：`/ui/weixin`（状态 / 重新扫码 / 白名单 / 删除凭据）。扫码只需把页面给出的
`liteapp.weixin.qq.com` 链接用手机微信打开——所以不渲染二维码图片，前端零依赖。

### 运维小工具

```bash
aiknow-server weixin send-text "文本"      # 往最近一次会话发一条文本（排查触达用）
aiknow-server weixin send-image ./pic.png  # 发一张图片
```

图片走 CDN + AES-128-ECB 加密上传（`pkg/ilink/SendImage`）。注意两个编码别搞混：
`getuploadurl` 的 `aeskey` 是 **hex(32)**，`image_item.media.aes_key` 是 **base64(hex)**；
用反了 CDN 返回 500 且不带错误信息。实测微信**不提供**对收到的图片「长按识别二维码」
（只有摄像头扫码），所以别指望用这条链路完成扫码登录。

CLI（与页面等价）：

```bash
aiknow-server weixin login      # 打印二维码链接，手机微信打开授权，凭据落库
aiknow-server weixin login --allow-from "a@im.wechat,b@im.wechat"
aiknow-server weixin status     # 看 bot_id / 白名单 / 游标 / 最近活跃
aiknow-server weixin logout     # 删除本地凭据
```

## 6. 关键设计决策

1. **游标落库。** 三个参考实现（[lib-x/ilink](https://github.com/lib-x/ilink)、
   [etng/weixin-bot-go](https://github.com/etng/weixin-bot-go)、
   [corespeed-io/wechatbot](https://github.com/corespeed-io/wechatbot)）都把
   `get_updates_buf` 放在内存变量；内嵌到 aiknow 后每次发版/迁移/改配置都重启，
   内存游标归零 = 重收旧消息、重复入库、重复回复。
2. **凭据入库不出现在配置里。** `bot_token` 是扫码动态产生的 L3 凭据，
   写进 `.env` 会随部署物料进 git、进备份。
3. **at-least-once + 内存去重。** 先处理消息、再推进游标：顺序反了崩溃即丢消息；
   重复投递由 `message_id` 去重（进程内 500 条，重启窗口内可能重放一次）。
4. **白名单默认「仅扫码者本人」。** 空 `allow_from` 不是「放行所有人」。
   一条消息可能触发无头浏览器渲染 + LLM 调用，代价远高于普通 API。
5. **bot 与主链路隔离。** 会话过期（`-14`）只让 bot 自己退出并打印提示，
   不影响 HTTP/队列；关掉 `WEIXIN_BOT_ENABLED` 即彻底停用。
6. **采集失败不挡后续。** 采集到正文即入库，URL 文章由 worker 在处理时
   重新采集（渲染 + 截图），与后台录入路径完全一致。

## 7. 已知限制

- **群聊不可用（已实测）**：微信不允许把 ClawBot 拉进群，因此 `group_id` 只可能是
  空串——协议里存在该字段（以及 `at_bot_username_list`）只说明协议层预留了群聊能力，
  **不代表产品上能用**。代码保持「`group_id` 非空即忽略」，将来若微信开放入群，
  仍需补 @ 判定（`at_bot_username_list`）与群级白名单才能真正启用。
  > 曾据逆向文档与 cc-connect 文档推断「群聊可做」，属把协议字段当成产品能力，
  > 已被实测否定。重开此议题前先确认 bot 能被拉进群。
- **主动推送有窗口限制**：回复必须带该会话最近的 `context_token`。已完成「处理完成
  回推」（见 4.1），但腾讯未公开窗口时长；实测历史 token 在十几分钟后仍可发送，
  长延迟仍可能被静默丢弃——排查看第 8 节。
- **只处理文本**：图片/语音/文件不解析（语音转写文本若随 `voice_item` 下发，
  作为文本处理）。
- **会话过期必须重新扫码**：`-14` 不可自动恢复。
- **多副本会抢消息**：`get_updates_buf` 是单游标，bot 只能跑在一个副本上
  （当前实现随 `serve` 启动，多副本部署需改为独立进程或加选主）。

## 8. 排障

| 现象 | 排查 |
|---|---|
| `尚未扫码登录` | 跑 `aiknow-server weixin login` |
| `微信 bot 已停止: session expired` | token 失效，重新 `weixin login` |
| 发消息没反应 | 先看日志三行：`inbound message` → `sending reply` → `reply accepted`。三行齐全但微信端没显示，属**下行静默丢弃**（见下）；缺 `inbound` 则是消息没到 bot（改白名单/会话绑定） |
| 链接采集失败 | 看日志 `collect failed`；公众号需 http 可达，渲染开启时更慢 |
| 重复入库 | 链接尾巴未规范化；`normalizeURL` 只去 fragment 与常见跟踪参数，其余需人工确认 |

### 下行静默丢弃（iLink 已知问题）

`sendmessage` 返回 `ret:0` + `message_id` 但微信端没显示，社区已有多例（账号级、
contextToken 过期、长时间 agent turn 后）。本项目实测：**首次消息可能不投递，
重发即正常**，且服务端对「已接受未投递」不会报错。

因此 `pkg/ilink` 在成功时打 `ilink: reply accepted message_id=...`：出问题时
用这三行日志区分「没收到」「没发出」「发了没到」。回复时间取入站后 1-2s
（关键词路径）是最稳的；URL 采集路径因为要渲染，第二条回执可能延后 10-20s。
