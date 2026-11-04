# 录入渠道总览

AiKnow 只有一个采集实现（`service.Collector`）和一个入库入口
（`ArticleService.CreateFromURL`），下面六个渠道都收敛到它，所以**去重、SSRF、
截图归档、分块、AI 处理的行为完全一致**。

| 渠道 | 场景 | 操作步数 | 需要什么 |
|---|---|---|---|
| 微信 Bot（ClawBot） | 手机上，想立刻入库 | 复制链接 → 粘贴发送 | 见 [WEIXIN_BOT.md](WEIXIN_BOT.md) |
| **浏览器扩展**（文件传输助手） | 手机上看到公众号文章，电脑上自动收 | 微信内「转发 → 文件传输助手」 | `extensions/aiknow-clipper` + 电脑开着网页版 |
| **浏览器扩展**（任意网页剪藏） | 电脑上看到好文章 | 右键 / 点图标 | 同上 |
| iOS 快捷指令 / Android 分享 | 手机上不想切应用 | 复制链接 → 触发快捷指令 | 见下文 |
| REST API | 脚本、自动化 | — | API Key（readwrite） |
| MCP | Claude 等外部 Agent | — | API Key（readwrite） |

## 为什么需要「文件传输助手」这条路

公众号文章的卡片**不能转发给 ClawBot**——它是插件会话，不在微信的「转发给朋友」
列表里。但「文件传输助手」是普通联系人，转发是微信原生能力。

实测（2026-11）网页版 `filehelper.weixin.qq.com` 收到的卡片 DOM：

```html
<a class="msg-appmsg__content"
   href="https://mp.weixin.qq.com/s?__biz=...&mid=...&idx=1&sn=...&mpshare=1&srcid=...#rd">
  <h4 class="msg-appmsg__title">文章标题</h4>
</a>
```

带真实 `href` 和标题，所以扩展能抽链接入库。安装与限制见
[extensions/aiknow-clipper/README.md](../extensions/aiknow-clipper/README.md)。

### 实现时踩到的四个坑（真机验证）

| 坑 | 说明 |
|---|---|
| **登录后换域名** | `filehelper.weixin.qq.com` → **`szfilehelper.weixin.qq.com`**；扩展只匹配前者会静默失效 |
| **刷新即空列表** | 刷新后只剩引导文案；但**下一条新消息到达时历史整批回填**（实测 0 → 8 条，含刷新前发的）。所以「打开页面补录」是主路径 |
| **节点整批重建** | 不是逐条 append。必须全量扫描 + 已见去重，只监听新增节点会漏 |
| **`skey` 在图片 URL 里** | 头像/封面 `img src` 带网页会话凭据，只发 `href` |
| **登录二维码约 25 秒就换一张** | 每次换码后旧码失效。手工扫码无感，但任何"取码 → 转发给人 → 扫码"的自动化链路都会失败（实测连试 5 次全部扫到过期码） |
| **登录态随页面实例销毁**：切走页面 **8 秒**再回来就退回扫码页 | 关掉/刷新/切标签页都要重扫 |
| **登录前转发的消息不会补录** | 实测：重新登录后 0 条；登录后发新消息只回填「登录之后」的；刷新后由页面自己发消息也只回填登录后的 | **只有「页面已登录且一直开着」时转发的消息才收得到** |

> **结论（重要）**：这条路只能当**常驻标签页**时的辅助通道，不能当主力。
> 「早上关掉、晚上打开」的场景下，期间转发全部丢失，且每次都要重扫。
> 手机端主力请用 iOS 快捷指令，PC 端主力用剪藏扩展。

页面本身是网页版微信协议的裁剪版（`webwxinit` / `webwxsync` / `synccheck`），
随时可能被腾讯关停——所以它只能当辅助通道，不能当唯一入口。

## iOS 快捷指令（手机端最省事）

手机主场景、又不想依赖电脑时，用「读剪贴板 → POST」的快捷指令：

1. 快捷指令 App 新建，动作顺序：
   - `获取剪贴板`
   - `获取 URL 内容` → 方法 `POST`
   - URL：`https://<你的 aiknow 域名>/api/v1/articles/from-url`
   - 请求头：`Authorization: Bearer <API Key>`、`Content-Type: application/json`
   - 请求体（JSON）：`{"url": "<剪贴板内容>"}`
2. 绑定到「轻点背面两下」或锁屏小组件。

之后在微信里「复制链接」→ 双击背面 → 入库完成，**不用切应用**。

## URL 归一与去重（所有渠道共用）

`pkg/urlx.Normalize` 负责让「同一篇文章」在库里只有一条：

| 处理 | 说明 |
|---|---|
| 去掉 `#rd` 等片段 | 微信分享必带 |
| 剔除分享态参数 | `mpshare`、`srcid`、`sharer_shareinfo`、`sharer_shareinfo_first`、`scene`、`ascene`、`subscene` |
| 剔除营销参数 | `utm_*`、`spm`、`from`、`clicktime` … |
| 公众号重排 | 只保留 `__biz`、`mid`、`idx`、`sn`、`chksm` 并按固定顺序编码 |

结果示例（转发两次，只有 srcid 不同）：

```
原始：mp.weixin.qq.com/s?__biz=MzY...&mid=2247&idx=1&sn=ed93&mpshare=1&scene=1&srcid=AAA#rd
      mp.weixin.qq.com/s?__biz=MzY...&mid=2247&idx=1&sn=ed93&mpshare=1&scene=1&srcid=BBB
归一：mp.weixin.qq.com/s?__biz=MzY...%3D%3D&chksm=...&idx=1&mid=2247&sn=ed93   ← 完全相同
```

## 安全约定

- **扩展只发链接，不发 DOM**：网页版的头像/封面 `img src` 里带 `skey=@crypt_…`，
  那是网页会话凭据，发给服务端或写进日志都等于泄露登录态。
- **API Key 存浏览器本地**（`chrome.storage.local`），不要写进扩展源码；
  扩展代码可被解包，建议给剪藏单独建一个 Key，便于随时吊销。
- 服务端采集统一过 `pkg/ssrf`：内网地址、非白名单端口、非 http(s) 协议一律拒绝。
