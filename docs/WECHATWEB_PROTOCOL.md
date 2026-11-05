# 文件传输助手网页版协议（实测版）

> 本文是**实机抓包核实后**的协议说明，不是推测。抓包方式见第 10 节。
> 与网上流传的 `wx.qq.com` 版文档有 7 处关键差异（第 9 节），照抄旧文档必然踩坑。
>
> 范围：**只覆盖「收」**——登录、初始化、长轮询、增量拉取、消息解析。
> 发送接口（`webwxsendmsg` 等）仅在第 7 节列出，本项目**刻意不实现**（原因见 7.3）。

## 1. 常量

| 项 | 值 |
|---|---|
| `appid` | `wx_webfilehelper`（**不是**通用网页版微信的 `wx782c26e4c19acffb`） |
| 登录域名 | `login.wx.qq.com`（备 `login.wx2.qq.com`） |
| 二维码域名 | `login.weixin.qq.com` |
| 业务域名 | `szfilehelper.weixin.qq.com`（登录后切到 sz 前缀） |
| 必需请求头 | `mmweb_appid: wx_webfilehelper` |
| UA | 任意现代 Chrome UA |

**`mmweb_appid` 头是全局硬要求**：所有 `/cgi-bin/mmwebwx-bin/*` 请求都要带，
漏了返回 `{"BaseResponse":{"Ret":1102}}`，**没有 ErrMsg**，极难排查。

## 2. 登录

### 2.1 申请 uuid

```
GET https://login.wx.qq.com/jslogin
  ?appid=wx_webfilehelper
  &redirect_uri=<双重 urlencode 的 https://szfilehelper.weixin.qq.com/cgi-bin/mmwebwx-bin/webwxnewloginpage>
  &fun=new&lang=zh_CN&_=<毫秒时间戳>
```

返回 JS 文本：`window.QRLogin.code = 200; window.QRLogin.uuid = "xxx";`

> `redirect_uri` 是**双重编码**（`https%253A%252F%252F...`），照抄单层编码会失败。

### 2.2 二维码

```
https://login.weixin.qq.com/qrcode/<uuid>    # 图片，可渲染/可被微信扫码
https://login.weixin.qq.com/l/<uuid>         # 微信内可直接打开的长链接
```

有效期由服务端控制（实测 4-5 分钟）。网页版把码每 **25 秒**换一次，这是页面行为，
自己调 `jslogin` 拿到的码不会那么快过期。

### 2.3 轮询扫码状态

```
GET https://login.wx.qq.com/cgi-bin/mmwebwx-bin/login
  ?loginicon=true&uuid=<uuid>&tip=1&r=<随机数>&_=<毫秒>&appid=wx_webfilehelper
```

返回 `window.code=408;` / `window.code=201;` / `window.code=200;`，`200` 时还有
`window.redirect_uri="https://...";`。

| code | 含义 | 动作 |
|---|---|---|
| 408 | 未扫码 | 继续轮询（1.5s 间隔足够） |
| 201 | 已扫码待确认 | 继续轮询 |
| 200 | 已确认 | 用 `redirect_uri` 换凭据 |

### 2.4 换凭据

```
GET <redirect_uri>&fun=new
```

响应是 XML：

```xml
<error>
  <ret>0</ret><message></message>
  <skey>@crypt_xxxx</skey>
  <wxsid>xxxxxxxx</wxsid>
  <wxuin>144662368</wxuin>
  <pass_ticket>xxxxxxxx</pass_ticket>
  <isgrayscale>1</isgrayscale>
</error>
```

同时下发 cookie。**需要持久化的 cookie（实测 8 个）**：

```
wxuin, wxsid, webwx_data_ticket, webwx_auth_ticket, webwxuvid, wxpluginkey, wxloadtime, mm_lang
```

`webwx_data_ticket` 只在图片上传时用到（本项目不发送，可只存不用）。

## 3. 初始化（每个会话只做一次）

```
POST https://szfilehelper.weixin.qq.com/cgi-bin/mmwebwx-bin/webwxinit
  ?r=<毫秒>&lang=zh_CN&pass_ticket=<pass_ticket>
Header: mmweb_appid: wx_webfilehelper
Content-Type: application/json
```

```json
{"BaseRequest": {
  "Uin": "144662368",          // 字符串！webwxsendmsg 里却是数字，混用会 1102
  "Sid": "<wxsid>",
  "SKey": "<skey>",
  "DeviceID": "123456789012345" // 15 位纯数字，无 "e" 前缀，每次可不同
}}
```

响应需取：

| 字段 | 用途 |
|---|---|
| `User.UserName` | 自己的 UserName（留档/校验） |
| `User.NickName` | 展示用 |
| `SyncKey{Count,List[]}` | 增量游标（网页版首次 4 项） |
| `SyncCheckKey` | 长轮询游标（实测与 SyncKey 内容相同，仍建议都存） |

> **只做一次**：网页版刷新页面会重新 `webwxinit`，SyncKey 被重置，历史消息就拿不回来了。
> 服务端要长期挂，绝不能反复 init——这也是"服务端能补离线消息、页面不能"的根本原因。

## 4. 收消息

### 4.1 长轮询 synccheck

```
GET https://szfilehelper.weixin.qq.com/cgi-bin/mmwebwx-bin/synccheck
  ?r=<毫秒>&skey=<skey>&sid=<wxsid>&uin=<wxuin>
  &deviceid=<任意 15 位数字>
  &synckey=1_903615827|2_882327758|3_903615757|...   # 取自 SyncCheckKey
  &mmweb_appid=wx_webfilehelper
```

返回：

```js
window.synccheck={retcode:"0",selector:"0"}
```

| selector | 含义 |
|---|---|
| 0 | 无新消息 |
| 2 | 有新消息（最常见的"有消息"值） |
| 7 | 有新消息（另一变体，实测出现过） |
| 其它非 0 | 有新消息，统一按"去拉"处理 |

**实测节奏**：空闲时服务端 **hold 约 25 秒**才返回 `selector=0`；有消息时立即返回。
所以客户端不需要 sleep，拿到 0 立刻发起下一次即可 —— 平均请求频率 ≈ 2.4 次/分钟。

> 网页版是用 `<script src=...>`（JSONP）发这个请求的，所以浏览器的 fetch/XHR 钩子
> 抓不到它。服务端直接 `GET` 读文本即可，不需要执行 JS。

### 4.2 拉增量 webwxsync

仅在 `selector != 0` 时调用：

```
POST https://szfilehelper.weixin.qq.com/cgi-bin/mmwebwx-bin/webwxsync
  ?sid=<wxsid>&skey=<skey>&pass_ticket=<pass_ticket>
Header: mmweb_appid: wx_webfilehelper
```

```json
{
  "BaseRequest": {"Uin": 144662368, "Sid": "...", "SKey": "...", "DeviceID": "..."},
  "SyncKey": {"Count": 13, "List": [{"Key":1,"Val":903615827}, ...]},
  "rr": -1793888128
}
```

响应顶层字段：

```
BaseResponse, AddMsgCount, AddMsgList[],
ModContactCount, ModContactList[], DelContactCount, DelContactList[],
ModChatRoomMemberCount, ModChatRoomMemberList[],
Profile, ContinueFlag, SyncKey, SKey, SyncCheckKey
```

**游标滚动**：把响应里的 `SyncKey` 整体覆盖旧值。实测会从 4 项增长到 13 项
（新增 `11/23/24/25/27/201/203/206/1000/1001`）。漏掉这一步会导致重复或丢消息。
响应里也有 `SKey` 字段，本项目的实现不依赖它轮换（未观察到变化）。

## 5. 消息结构

### 5.1 通用字段（AddMsgList 元素）

| 字段 | 说明 |
|---|---|
| `MsgId` | 消息 ID，**去重首选** |
| `NewMsgId` | 新版 ID（数值），备用 |
| `MsgType` | 1 文本 / 3 图片 / 34 语音 / 49 链接卡片 / 51 状态通知 / 10000 系统 |
| `AppMsgType` | `MsgType=49` 时细分：5 = 链接/公众号文章 |
| `FromUserName` | 发送者（同一账号多端时也是自己，见 5.4） |
| `ToUserName` | 对文件传输助手恒为 `filehelper` |
| `Content` | 正文或 XML（编码规则见 5.2） |
| `CreateTime` | 秒级时间戳 |
| `Status` / `ImgStatus` | 状态位，一般忽略 |

### 5.2 ⚠️ Content 的编码规则（最容易踩的坑）

实测 `MsgType=49` 的 `Content` 形如：

```
&lt;?xml version="1.0"?&gt;<br/>	&lt;msg&gt;<br/>		&lt;appmsg&gt;<br/>
...&lt;url&gt;https://mp.weixin.qq.com/s?__biz=X&amp;amp;mid=1...&lt;/url&gt;...
```

三个特征同时存在：

1. 整段 XML 被 **HTML 转义了一层**（`&lt;` → `<`）；
2. 行间插入了**真实的 `<br/>` 标签**（不是转义），以及 `\t` 缩进；
3. XML 内部 URL 的 `&` 是 `&amp;`，再被外层转义成 `&amp;amp;`（**两层**）。

**正确解码顺序**：

```
1) 删除所有 <br/>            （真实标签，留着 XML 解析器会报错）
2) HTML 反转义 一层           （解完才是合法 XML，URL 里仍是 &amp; 实体）
3) 交给 XML 解析器            （它负责把 &amp; 还原成 &）
```

**不要**直接反转义两层再解析：那样 URL 里会留下裸 `&`，XML 立刻非法。
若遇到只转义了一层的消息，做法是"解析失败后再多解一层重试"（本项目的
`parseAppMsg` 就是这么写的）。

### 5.3 公众号文章卡片（`MsgType=49`, `AppMsgType=5`）

解码后的 XML（关键字段）：

```xml
<msg>
  <appmsg>
    <title>Droidline：不用ADB的开源安卓自动化工具</title>
    <type>5</type>
    <url>https://mp.weixin.qq.com/s?__biz=...&amp;mid=...&amp;idx=1&amp;sn=...&amp;mpshare=1&amp;scene=1#rd</url>
    <sourcedisplayname>码农知道的事</sourcedisplayname>
    <appattach>
      <cdnthumburl>305f020100...</cdnthumburl>
      <cdnthumbaeskey>084386...</cdnthumbaeskey>
      <cdnthumbwidth>150</cdnthumbwidth><cdnthumbheight>150</cdnthumbheight>
    </appattach>
    <webviewshared><publisherId>...</publisherId></webviewshared>
  </appmsg>
</msg>
```

要点：

- `url` 带分享态参数（`mpshare`/`scene`/`srcid`/`sharer_shareinfo`/`#rd`），
  **入库前必须归一**，否则同一篇文章每次转发都是一个"新链接"。
- 封面图在 `appattach.cdnthumburl`（+ `cdnthumbaeskey` 解密），不是必需要素。

### 5.4 其它类型

| MsgType | 内容 | 处理建议 |
|---|---|---|
| 1 | 纯文本；用户直接粘的链接也在这里 | 可从文本中提取公众号 URL |
| 3 | 图片。**`Url`/`MediaId` 为空**，元数据在 `Content` 的 XML 里（`cdnthumburl`/`aeskey`/`length`/`md5`）；取图用 `webwxgetmsgimg?MsgID=<MsgId>&skey=<skey>&type=slave` | 本通道不处理 |
| 34 | 语音（`VoiceLength`，内容 `@` 开头的加密串） | 忽略 |
| 51 | 状态通知/操作日志（首次同步常见） | 忽略 |
| 10000 | 系统提示 | 忽略 |

**自己发的消息不会回推到 `AddMsgList`**（实测发送后 sync 为空），所以不需要
"过滤自己发的"。文件传输助手本身也会发消息，所以会话内所有 `49`/`1` 都来自本人。

## 6. 错误码

| Ret | 含义 | 处理 |
|---|---|---|
| 0 | 成功 | — |
| 1100 / 1101 / 1102 | 会话不可用（未登录/被踢/凭证失效） | **停止，等人工扫码**，不要自动重登 |

`1102` 最常见的成因不是"过期"，而是**漏了 `mmweb_appid` 头**或 `Uin` 用错了类型
（init 用字符串、sendmsg 用数字）。先查这两项，再怀疑会话。

## 7. 发送（本项目不实现）

### 7.1 文本

```
POST /cgi-bin/mmwebwx-bin/webwxsendmsg?lang=zh_CN&pass_ticket=<...>
{"BaseRequest":{...},"Msg":{"Type":1,"Content":"...","FromUserName":"<自己>",
 "ToUserName":"filehelper","LocalID":"<ts<<4 + rand>","ClientMsgId":"<同上>"},"Scene":0}
```

### 7.2 图片（两步）

上传 `POST https://file.wx2.qq.com/cgi-bin/mmwebwx-bin/webwxuploadmedia?f=json&random=...`
（multipart，字段含 `uploadmediarequest` JSON、`webwx_data_ticket`），拿 `MediaId`；
再 `POST /cgi-bin/mmwebwx-bin/webwxsendmsgimg` 发送（`Type=3`, `Content` 空）。

### 7.3 为什么本项目不实现发送

- 业务不需要：入库只要 URL；
- **权限面**：`ToUserName` 是联系人 ID，协议上并不限定只能发给 `filehelper`。
  一旦实现发送，等于获得了"以账号身份给任意好友发消息"的能力，一旦被利用后果严重；
- 风控面：发送是这类逆向通道里最容易被识别、也最容易触发限制的动作。

因此代码里 `webwxsendmsg` / `webwxsendmsgimg` / `webwxuploadmedia` 一个都没有。
若将来真要发，`ToUserName` 必须硬编码为字面量 `filehelper`，且不开放配置。

## 8. 实测行为 / 风控建议

| 现象 | 实测结论 |
|---|---|
| 会话能活多久 | 网页版页面挂了一整天没掉（只要轮询在跑）；服务端同理 |
| 页面刷新 | 会重新 init → SyncKey 重置 → 历史拿不回 |
| 两个消费者 | 同时 sync 会**抢消息并重复投递**；必须独占 |
| 请求频率 | 空闲时每 ~25s 一次 synccheck、0 次 sync，与真人开网页版无异 |
| 二维码 | 自己取码 TTL 约 4-5 分钟；页面 25s 换码是页面行为 |
| 多端 | 网页版登录不会踢手机端，但会挤掉另一个网页会话 |

结论：**只收不发 + 复刻页面节奏 + 不自动重登**，行为特征与"人工开着网页版"基本一致，
是这类通道里风险最低的用法。仍需注意：逆向协议本身违反微信用户协议，建议用小号。

## 9. 与流行文档的 7 处差异

| # | 流行写法 | 实测 |
|---|---|---|
| 1 | `appid=wx782c26e4c19acffb` | `wx_webfilehelper` |
| 2 | 业务接口在 `wx.qq.com` | `szfilehelper.weixin.qq.com` |
| 3 | 未提 | 全局必需 `mmweb_appid` 头，漏了 `ret=1102` 无错误信息 |
| 4 | synccheck 当普通 GET | 页面用 `<script>` JSONP；返回 `window.synccheck={...}` |
| 5 | Content 直接当 XML | 两层 HTML 转义 + 真实 `<br/>` 折行，顺序解错就非法 |
| 6 | 图片消息有 MediaId | `Url`/`MediaId` 为空，元数据在 XML，取图走 `webwxgetmsgimg` |
| 7 | 发图 = `sendmsg`+`Type=3` | 独立端点 `webwxsendmsgimg` + 上传到 `file.wx2.qq.com` |

## 10. 如何自己复现这份抓包

1. `agent-browser network har start` + `agent-browser open --headed --init-script recorder.js https://filehelper.weixin.qq.com/`，
   扫码登录。
2. `recorder.js` 在页面 JS 之前 hook `fetch` 与 `XMLHttpRequest`，把
   `mmwebwx-bin/*` 的请求体与响应文本存进 `window.__wxlog`。
   `synccheck` 是 `<script>` 注入的，钩子抓不到——用 `agent-browser network requests --filter synccheck` 看 URL 与频率。
3. 想看完整的入站消息（包括卡片 XML）：登录后在控制台读
   `document.querySelector('#app').__vue_app__.config.globalProperties.$store.state.msgList`，
   `MsgType=49` 的元素里 `Content.msg.appmsg.url` 就是解析后的 URL。
4. 想验证"漏 header 会 1102"，删除 `mmweb_appid` 重放 `webwxinit` 即可复现。
