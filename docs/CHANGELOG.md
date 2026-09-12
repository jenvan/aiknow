# AiKnow 变更日志

## [Unreleased] - 2026-09-12

### feat: 检索测试页「关键词检索」增加分词开关（tokenized）

在后台「检索测试」页的关键词检索表单，关键词输入框右侧新增「分词」复选框，
对应 `KeywordSearchParams.Tokenized`（默认勾选=分词）。勾选时用 jieba 分词
（`|||`，"知识图谱"拆成"知识"/"图谱"，召回多篇）；取消勾选时用
`to_tsquery('simple')` 整词精确匹配（仅召回含完整"知识图谱"的文章）。
后端 `/search/debug/keywords` 路径已透传该参数，仅改前端
（`web/js/pages/search.js` 的 `kparams` 与 `web/index.html` 表单）。
已用 agent-browser 验证：分词模式召回 5 条、精确模式仅 1 条，与数据库基线
（16 vs 1）一致，均无报错。

### feat: MCP 检索工具补齐参数（category / tokenized / 统一 limit）

对齐后台「检索测试」页能力，为两个 MCP 查询工具补充参数：

- `search_knowledge` 新增 `category`（按分类名称过滤，大小写不敏感，可选），
  透传至 `SearchParams.Category`。
- `find_articles_by_keywords` 新增 `tokenized`（布尔，默认 true）：true 用
  jieba 分词（`|||`，"父母"可匹配"父母牵线"，BM25 打分）；false 用
  `to_tsquery('simple')` 整词精确匹配（"小日本"不拆分，按字段权重排序）。
  `KeywordSearchParams.Tokenized` 用 `*bool` 三态，nil 时默认分词，
  避免调用方漏传退化成精确模式（不影响检索测试页现有行为）。
- 两个工具的返回数量参数统一命名为 `limit`（原 `search_knowledge` 为 `top_k`）。

仓库层 `KeywordSearch` 按 `tokenized` 分支构造文本匹配 CTE，新增
`buildSimpleTSQuery` 将关键词列表构造为 `to_tsquery('simple')` 的 OR 查询串
（词素单引号包裹并转义，保留多字词整体）。已用 agent-browser 在 MCP 调试页
验证：tools/list schema 正确暴露新参数；`search_knowledge` 带 category 正常
返回；`find_articles_by_keywords` 关键词「知识图谱」分词模式召回多篇、精确
模式仅召回含整词的 1 篇，均无报错。

### fix: 检索测试页混合搜索的分类过滤报错 column reference "id" is ambiguous

`BM25Search` 的分类过滤子查询 `SELECT id FROM articles a JOIN categories c ...`
中 `articles` 与 `categories` 均含 `id` 列，导致 `id` 引用歧义
（SQLSTATE 42702）。改为显式 `SELECT a.id`。

### feat: 文章列表接入 BM25 全文检索 + 高级筛选面板

**后端**：`GET /api/v1/admin/articles` 的 `search` 参数由 `ILIKE title`
改为走 `articles_search_idx`（title/summary/content 三字段，jieba 中文分词），
保留 category/status/tag/date range/sort 等所有现有过滤条件。新增
`GET /api/v1/admin/categories` 和 `GET /api/v1/admin/tags` 供前端下拉加载。

**前端**：检索框改为全文检索（覆盖标题/摘要/正文）；「高级筛选」折叠面板
支持分类下拉、标签多选（chip 显示）、日期范围、多字段排序；表格新增
「更新日期」「阅读量」列。分页显示改为「第 X-Y 条，共 N 条」。

### feat: GET /api/v1/articles 支持按分类名称和标签名称过滤

新增可选参数 `category`（分类名称，大小写不敏感）和 `tags`（标签名称，
逗号分割，大小写不敏感），在 service 层通过 `categoryRepo.GetByName` /
`tagRepo.GetByName` 转为内部 ID 后传入 repository。现有 `category_id` /
`tag_id` 参数行为不变，两者可同时使用。

### feat: /api/v1/search 支持按 category 过滤

在 `POST /api/v1/search` 请求体中新增可选参数 `params.category`，
向量检索（VectorSearch）与 BM25 检索（BM25Search）均通过 JOIN articles
关联分类后按名称过滤，大小写不敏感。未传时行为不变。同步更新
`DebugSearch` 与 `SearchWithConcurrent` 两个入口。

### 前端资源本地化：Tailwind 与 Alpine 去 CDN
- 将 Tailwind Play CDN 脚本与 Alpine.js 3.x 官方发布包下载到本地
  `web/js/vendor/`（`tailwind.js` / `alpine.cdn.min.js`），`index.html` 与
  `login.html` 改引用本地路径，不再依赖 `cdn.tailwindcss.com` / `jsdelivr`。
- 运行行为不变（本地仍是 Play CDN 运行时脚本）；`StaticFS("/web")` 已正确服务
  两个文件（`text/javascript`）。已用 agent-browser 验证登录页/仪表盘渲染、
  Alpine 组件注册正常。
- 说明：Play CDN 脚本自带 production warning（console 提示"应改用 Tailwind CLI"），
  属预期；如需消除可另立任务改为静态 CSS 编译。

### docs: 完善 docs/API.md，新增第三方调用说明

在原有简短接口列表基础上补全为完整对外 API 文档，覆盖：
- API Key 鉴权与 scope 权限模型说明
- 文章管理（创建/列表/详情/软删除）及 URL 采集两种来源的用法
- 混合检索与关键词检索的完整请求参数与响应字段
- API Key 创建/启用/禁用/删除接口（管理后台）
- MCP 工具接入入口
- 快速开始 curl 示例

### docs: 新增 docs/API_ADMIN.md，补录后台管理接口

将 API.md 中的后台管理接口（Session 鉴权部分）抽取为独立文档 API_ADMIN.md，
覆盖：认证流程、文章 CRUD/预览/批量操作、API Key 管理、仪表盘、
检索调试、MCP 调试代理、日志、系统配置、提示词管理等全部接口及快速上手示例。
API.md 保留对 API_ADMIN.md 的交叉引用。

## [Unreleased] - 2026-09-10

### 布局：仪表盘下方改为「左三块 + 右最近录入」
- 上方四个统计卡片不变。
- 下方由原来「2×2 网格（状态分布/队列/健康/最近录入）」改为左右两列：
  左列依次 **服务健康 → 任务状态 → 任务队列**；右列整块 **最近录入**，
  `self-stretch` 撑满左列三块总高。
- 纯前端：`web/index.html` 的 `dashboardPage` 模板。`dashboard.js` 未动。

### 功能：文章编辑页展示采集来源网址

**背景**：URL 采集入库的文章带 `source_url`，但编辑页 `/ui/articles/:id` 未展示，
编辑者不知道正文来自哪个链接。

**改动**：编辑页（`web/index.html` 的 `articleEditPage` 模板）在标题输入框上方加
「来源网址」条（蓝色提示块），`x-show="article && article.source_url"`——仅 URL 采集的
文章（有 `source_url`）显示，手动录入不显示。链接 `target="_blank" rel="noopener"`。
数据链路已具备：`GET /articles/:id` 返回整篇 article（含 `source_type`/`source_url`），
前端无需改 JS。

- 变更：`web/index.html`

### 功能：后台 UI 重组 + 品牌化 + 未登录直接跳转登录

**背景**：后台左侧菜单 8 项平铺、视觉陈旧（深色侧栏 + 每页重复大标题）；
品牌未统一；未登录用户打开 `/ui/*` 会停留在一个「未登录」状态页，还需多点一次「登录」。

**菜单重组**（`web/js/app.js` 的 `NAV_ITEMS` → 按功能域分 4 组）：
- 概览（仪表盘）· 内容（文章列表/录入文章）· 检索 & 调试（检索测试/MCP 调试/MCP 日志）·
  系统（系统配置/API Key）。URL/路由不变，仅侧栏展示分组（组小标题 + 组内项 + inline-SVG 图标）。

**布局与视觉**（`web/index.html`，Tailwind CDN + Alpine，零构建、不引新依赖）：
- 新增**顶部 header bar**：左=当前页标题，右=用户头像/名 + 退出；侧栏底部用户/退出上移至此。
- 侧栏改**白底** + 品牌蓝 accent（当前项浅蓝底 + 左侧 3px 强调条 + 蓝字图标）；各页**移除重复的
  页内大标题**（header 已显示页名）。
- **响应式**：桌面固定侧栏；窄屏（`lg` 以下）侧栏收进 header 的汉堡抽屉 + 遮罩。

**品牌化**：
- 品牌名 **AiKnow**，中文 **爱脑**，slogan **AI 知识库管理工具**；两处 `<title>` 同步。
- logo 换成「神经元网络」图标（蓝紫渐变），贴「AI 知识库」语义；侧栏 + 登录页共用。

**未登录直接跳转**：`web/js/app.js` 的 `shell.init()`——`fetchSession()` 无会话时
`location.replace('/ui/login')`，不再渲染「未登录」状态页（省一次点击）。已删 index.html
里的未登录停留模板。

- 变更：`web/js/app.js`、`web/index.html`、`web/login.html`
- 说明：纯前端改动，后端路由/各页 `pages/*.js` 逻辑与 `x-if` 页面块均未动；已用
  agent-browser 逐页验证（8 页 + 登录 + 未登录跳转 + 窄屏）无 Alpine 渲染破损。

### 修复：URL 采集标题获取（微信等 <title> 为空的页面取不到标题）

**现象**：`add_article_by_url` 采集微信文章时标题取不到，回落 "Untitled"。

**根因**：`Collector.extractContent`（`internal/service/collector.go`）此前只从
`<title>` 标签取标题，而微信（mp.weixin.qq.com）等页面的 `<title>` 常为**空**，
真实标题放在 `og:title` meta（部分站点在 twitter:title / `#activity-name`）。

**修复**：新增 `extractTitle(html)`，按优先级 `og:title → twitter:title →
#activity-name → 首个 <h1> → <title>` 取标题（用 goquery，仓库已有依赖），取到即停，
全部落空才回落 "Untitled"。

- 变更：`internal/service/collector.go`
- 测试：`internal/service/collector_test.go` 新增 `TestExtractTitle`（覆盖微信
  "title 空 + og:title 有值"、各优先级、全空回落）。已用真实微信页验证能取到
  「一篇讲透Agent自进化飞轮…」标题。

### 修复：系统配置中字符串型配置项（如 llm.model）无法编辑设置

**现象**：后台系统配置页编辑 `llm.model` 等**字符串型**项时设不了——输入模型名后
保存报「JSON 格式有误」。

**根因**：`web/js/pages/settings.js` 的编辑框此前对所有配置项一律预填
`JSON.stringify(value)`、保存一律 `JSON.parse`。字符串项（默认空串）预填成 `""`
（带引号），用户直接敲 `qwen-plus` 不带引号，`JSON.parse` 失败 → 报错，表现为"设不了"。
数字型项（max_tokens 等）不受影响，故问题只在字符串项（llm.model /
collector.user_agent / prompt.analyze）。

**修复**：按值类型区分编辑——`typeof value === 'string'` 走**裸文本**编辑（`save`
直接取输入值，空串合法=回落默认），其余仍走 JSON。字符串项改用 `<textarea>`
（`prompt.analyze` 多行提示词可直接编辑）。

- 变更：`web/js/pages/settings.js`、`web/index.html`
- 说明：后端 PUT 契约此前已修复（`{ value: ... }`），本次仅前端编辑体验。

### 功能：采集正文字节上限后台可配（`collector.max_content_length`）+ 系统配置分组重排

**背景**：URL 采集在正文过长时按上限截断（提示「已采集 N 字（内容过长已截断）」），
但上限只来自 L1 启动配置 `DefaultCollectorConfig`（默认 2MB），后台改不了，页面被截断
只能改代码/重启。

**新增 L2 热配置 `collector.max_content_length`**：
- 走与 `collector.user_agent` 完全一致的「L1 启动配置设基线 + 后台覆盖、每次抓取
  热生效」模式：`Collector` 新增 `effectiveMaxContentLength`，每次 `Fetch` 读取
  `collector.max_content_length`，>0 则覆盖、0/未设回落启动默认（2MB）。
- 后台「系统配置 → 网页采集」新增该项，页面被截断时可在线调大，无需重启。
- `L2Config.Collector` 加 `MaxContentLength`，`cache.go` 加 apply case（与 user_agent
  接线一致）；前端靠遍历 `DefaultL2Config()` 自动渲染，无需新迁移（默认 0 = 用 2MB）。

**系统配置分组重排**（`web/js/pages/settings.js` 的 `GROUP_ORDER`）：
- 「提示词」提到最上面；
- 「文本分块」移到「网页采集」下面；
- 「MCP 日志」移到最后。
- 新顺序：提示词 → LLM → Embedding → Reranker → 混合检索 → 关键词检索 → 网页采集
  → 文本分块 → MCP 日志。

- 变更：`internal/service/collector.go`、`internal/model/setting.go`、
  `internal/config/cache.go`、`web/js/pages/settings.js`
- 测试：`internal/service/collector_test.go` 新增
  `TestCollectorEffectiveMaxContentLength`（无覆盖用启动默认 / 后台覆盖优先 / 0 回落）。

### 功能：结构优先的分块策略（空行+标题切段、超窗滑窗兜底、min_size 合并）

**背景**：原切分是纯字符滑窗（`chunk.size`/`chunk.overlap`），不看段落/标题/
空行结构。对「分段问答 / FAQ / 条款」这类数据不友好——会把相邻问答缝进同一
chunk、或把单个问答拆散，稀释向量检索、引用不干净。

**新策略 `structure`（默认）**：
- **按「空行」与「标题行（`#`…`######`）」切段**，段内保留原始换行；标题与其
  后正文同段，直到下一个空行/标题再分。段长 ≤ `chunk.size` → 整段一片；
  **超过 → 仅对该段滑窗兜底**（`chunk.size`/`chunk.overlap`）。
- **段落边界不重叠**（`chunk.overlap` 默认 0，只在超长段落滑窗时生效）。
- **`chunk.min_size`（默认 60）合并过短片段**：小于阈值的段（如采集里 `<li>`
  短列表项）并入前一片（合计不超 size），防止碎段稀释检索。
- 分隔符用「≥1 个空行」而非「2+ 换行」：兼容手工 FAQ（段间 ≥1 空行）与
  URL 采集正文（`htmlclean` 每块级元素前后写 `\n`，段落边界恰好是**单个空行**）。

**保留 `window` 策略**：`chunk.strategy=window` 走原纯滑窗行为，向后兼容；
`structure` 下无空行无标题的正文自动退化为滑窗，结果与原实现一致。

**收敛为唯一切分实现**：
- 新增 `internal/service/chunker.go`（`Split(content, ChunkConfig)` + 策略常量），
  `AIPipeline.SplitContent` 与（此前从未被调用、行为已分叉的）`chunkServiceImpl.
  SplitContent` 均委托到 `Split`，消除双实现。
- `ChunkConfig`{Strategy, Size, Overlap, MinSize}；内置默认 = structure/400/0/60。

**后台可配（L2 热生效，无需迁移）**：`chunk.strategy` / `chunk.size` /
`chunk.overlap` / `chunk.min_size`。系统配置页按 `DefaultL2Config()` 结构自动
分组展示，四项出现在「文本分块」分组下；改后对**重处理**的文章生效。

- 变更：`internal/service/chunker.go`（新增）、`internal/service/ai_pipeline.go`、
  `internal/service/chunk.go`、`internal/model/setting.go`、
  `internal/config/cache.go`、`web/js/pages/settings.js`
- 测试：`internal/service/chunker_test.go`（FAQ 一问一答一块 / 标题同段 / 超窗滑窗
  兜底含 50 字重叠 / min_size 合并与不合并 / 无空行退化滑窗 / window 策略 / 空正文），
  `internal/service/ai_pipeline_test.go`（死循环回归改走 `Split`）。

### 修复：系统配置页「编辑保存」写路径失效（前端未按后端契约包 value 层）

**现象**：后台系统配置页点「编辑」改任意 L2 值再「保存」，报 `配置值须为合法
JSON`，编辑框收不起来、值未落库，L2 热生效形同虚设。

**根因**：前后端契约不一致。后端 `UpdateSetting`（`internal/handler/admin/setting.go`）
要求请求体形如 `{"value": <任意JSON>}`，只存内层 value；而前端 `save()`
（`web/js/pages/settings.js`）发的是**裸值**（如 `2500`），被后端 `ShouldBindJSON`
按 `{"value":...}` 结构解析失败而 400。GET / 历史 / 单项重置 / 全部重置不受影响。

**修复**：`web/js/pages/settings.js` 的 `save()` 改为
`api.put('/settings/'+key, { value: parsed })`，与后端契约对齐。

- 变更：`web/js/pages/settings.js`
- 验证：agent-browser 端到端走通「编辑 llm.max_tokens → 保存 → 出现『已保存』
  + 行标记『已改』 + 编辑框收起」；DB `GET /settings/llm.max_tokens` 落库、
  reset 可回落默认。

### 修复：BAAI Reranker 得分恒为 0（字段名不匹配）且 top_k 未生效

**现象**：混合检索（向量 + BM25 + RRF + 精排）精排结果 `score` 全为 `0.0000`，
且返回条数超出 `top_k`。

**根因**：`internal/service/provider/reranker_baai.go` 解码的是 `score` 字段
（jina/cohere 风格），但所用 BAAI 兼容网关（siliconflow 的 `/v1/rerank`）
返回字段是 `relevance_score` → 每次解出 0；同时该网关忽略请求里的 `top_k`，
返回全部候选，而 provider 也未做本地截断。

**修复**：
- 响应结构体同时解析 `relevance_score` 与 `score`（指针区分缺省与为 0，取有值一侧）
- 精排后按得分降序排序，并本地截断到 `top_k`（`sort.SliceStable`）
- 新增回归测试 `internal/service/provider/reranker_baai_test.go`：
  锁住 `relevance_score` / `score` 两字段兼容，及网关忽略 top_k 时的本地截断。

- 变更：`internal/service/provider/reranker_baai.go`
  （新增 `internal/service/provider/reranker_baai_test.go`）
- 验证：真实调用 siliconflow 精排，得分变为真实相关性分（0.987/0.960/…，降序）
  且严格截断到 `top_k`；`go test ./internal/service/...` 全绿。

### 说明：agent-browser 全量后台验收

用 agent-browser（真实 Chrome）走通全部 8 个后台页 + 登录/写流程，暴露上述两处缺陷。

## [v0.6.2] - 2026-09-09

### 配置层级收紧：L1 默认值完整、L2 可运行时修改且优先级最高

**背景**：管理后台（L2 settings 表）此前混入两类不该出现的配置——与表结构/启动参数
绑定的固定配置（embedding.dimension、采集超时等），以及配置从未真正热生效
（ConfigCache.Reload 用 map 解析扁平标量必然失败，且 main.go 从未创建 ConfigCache）。

#### L1 启动配置补全（config.yaml 体现全部默认值）
- 网页采集超时/重定向次数/最小内容长度纳入 L1：
  `collector.timeout`（COLLECTOR_TIMEOUT，默认 10s）、
  `collector.max_redirects`（默认 3）、`collector.min_content_length`（默认 200）。
  理由：`http.Client` 在 `serve()` 启动时创建，运行时改不生效，放 L2 会制造"改了没生效"假象
- 变更：`internal/config/config.go`（managedKeys / defaultValues / YAMLConfig / toFlat）、
  `cmd/aiknow-server/main.go`、`config.yaml.example`、`.env.example`

#### L2 精简为真正可运行时修改的配置
- 从 `L2Config` 删除：`embedding.dimension`（与 VECTOR(1024) 表结构绑定，不可改）、
  `collector.timeout_ms / max_redirects / min_content_length`（移入 L1）
- 新增：`llm.model`（L2 覆盖 L1 QWEN_MODEL，实现后台在线切换模型）
- 迁移 `010_config_hierarchy_cleanup.sql`：删除上述 4 项、播种 `llm.model` 空值（=未覆盖）

#### L2 优先级机制真正接通（此前完全未使用）
- `ConfigCache.Reload` 重写：按各 key 已知类型直接解码扁平标量（数字 / JSON 字符串），
  修复"从未热生效"的根因；`applySetting` 补齐全部 L2 key
- `main.go`：创建 ConfigCache 并启动时 Reload；`SettingHandler` 配置变更（保存/重置）后
  通过 onChanged 回调触发 Reload——后台改值即时生效且优先级最高
- QwenProvider 支持运行时模型源（`SetModelSource`，Go 接口 `ModelSourceSetter`）：
  模型名优先级 = 调用方显式指定 > L2 `llm.model` > L1 `QWEN_MODEL` > 内置 qwen-plus
- 变更：`internal/config/cache.go`、`cmd/aiknow-server/main.go`、
  `internal/handler/admin/setting.go`、`internal/model/setting.go`、
  `internal/service/provider/llm.go`、`internal/service/provider/llm_qwen.go`

#### 各消费端接入 L2 配置（后台参数真正生效）
- 引入 `service.L2ConfigSource` 接口（`config.ConfigCache` 实现），各服务在每次调用时
  读取最新 L2 值，兑现「后台配置优先级最高且热生效」：
  - **AIPipeline**：LLM 参数（model/temperature/max_tokens，经 `GenerateJSON` 新增的
    `opts` 参数传入）、分块 `chunk.size` / `chunk.overlap`（替代硬编码 400/50）
  - **SearchService**：`search.*` 六项参数作为默认值；`reranker.top_k` 限制送入精排
    的候选数（控制成本）
  - **KeywordSearchService**：`keyword_search.*` 五项权重（此前仓库层硬编码 3.0/2.5/…）
  - **LogHandler**：`mcp_log.retention_days` 作为日志清理默认保留天数
- `GenerateJSON` 接口增加 `opts GenerateOptions` 参数（provider 与 `prompt.go` 同步更新）
- 变更：`internal/service/interfaces.go`、`search.go`、`search_keyword.go`、
  `ai_pipeline.go`、`prompt.go`、`internal/handler/admin/log.go`、`cmd/aiknow-server/main.go`、
  `internal/service/provider/llm.go`、`llm_qwen.go`
- `embedding.batch_size` 已入库可编辑，但当前 Embedding 逐条调用，暂无批量入口未消费

#### 系统配置页按功能分组展示（不再按首字母排序）
- 分组顺序固定，中文分组名（LLM / Embedding / Reranker / 混合检索 / 文本分块 /
  关键词检索 / MCP 日志 / 网页采集 / 提示词），每个配置项带取值范围说明
- 变更：`web/js/pages/settings.js`、`web/index.html`

#### 新增/调整测试
- `internal/config/cache_test.go`：扁平标量解码（字符串 / 数字）、未知 key 忽略
- `internal/service/provider/llm_qwen_test.go`：L2 模型源覆盖启动配置、空源回落
- `internal/service/l2config_test.go`：检索/关键词/AIPipeline/rerank 候选上限从 L2 读取
- `config_test.go`：示例配置新增收集器字段校验
- 新增 `docs/CONFIG_ARCHITECTURE.md` 记录配置分层设计与消费端接入清单

## [v0.6.1] - 2026-09-09

### 功能调整：LLM 模型名与采集 User-Agent 可配置

- **LLM 模型名（L1 启动配置）**：新增 `QWEN_MODEL`（config.yaml 的 `qwen.model`，默认
  `qwen-plus`）。此前真实 Qwen 调用发送的 model 字段为空串，无法通过 DashScope 鉴权。
  现在 `QwenProvider` 持有启动配置的模型名：`Generate` 未显式指定模型时回落该值，
  显式传入时优先。
  - 变更：`internal/config/config.go`（managedKeys / defaultValues / YAMLConfig / toFlat）、
    `internal/service/provider/factory.go`（L1ProviderConfig.LLMModel）、
    `internal/service/provider/llm_qwen.go`（NewQwenProvider 增加 model 参数）、
    `cmd/aiknow-server/main.go`
- **采集 User-Agent（config.yaml 设基线 + 后台可覆盖）**：新增 `COLLECTOR_USER_AGENT`
  （config.yaml 的 `collector.user_agent`，默认 `AiKnow/1.0 (Internal Knowledge Base)`）。
  后台「系统配置 → collector.user_agent」可覆盖并热生效：留空（或未设置）时回落启动
  配置值，实现双通道控制。
  - 变更：`internal/service/collector.go`（`SetUserAgentSource` / `effectiveUserAgent`，
    每次抓取实时读取 settings 表）、`internal/model/setting.go`（L2Config.Collector.UserAgent）、
    `internal/config/cache.go`（applySetting 支持 `collector.user_agent`）、
    `cmd/aiknow-server/main.go`、`internal/config/config.go`
- **管理后台系统配置页可用**：此前 10 个前端页面均为占位壳，系统配置页既未渲染也无法
  保存；app.js 还会轮询一个不存在的会话接口（`/admin/session/me`）导致登录态永远不可见。
  - 实现系统配置页：按 defaults 分组渲染全部 L2 配置项，逐项保存/恢复（含 CSRF Token），
    采集 User-Agent 即可在后台直接修改；修复 app.js 会话端点为 `/admin/auth/session`
  - 修复 `UpdateSetting`：此前把整个请求体 `{"value": ...}` 存入 settings 表，消费方
    读到的始终是外层对象（与迁移 005 播种的扁平标量不一致）；改为仅存内层 `value`
  - 变更：`web/index.html`、`web/js/pages/settings.js`（重写）、`web/js/app.js`、
    `internal/handler/admin/setting.go`
- 同步更新：`config.yaml.example`（移除「LLM 模型名固定不可配」的暗示）、`.env.example`、
  AGENT.md「核心配置参数」表
- 新增测试：`internal/service/provider/llm_qwen_test.go`（模型回落与显式覆盖）、
  `internal/service/collector_test.go`（User-Agent 覆盖优先 / 空覆盖回落 / 无覆盖回落）、
  `internal/handler/admin/setting_test.go`（UpdateSetting 存内层 value）

## [v0.6.0] - 2026-09-09

### Phase 1：鉴权闭环 + 后台管理全部后端接口 + 10 个前端页面

**本版本完成了 v0.5.8 未修复的三项遗留：对外 API 鉴权、API Key 落库、admin handler
全部实现。同时补齐了 10 个前端页面，并在此过程中修复了多处设计与实现缺陷。**

#### 鉴权体系

三套鉴权全部接线，与 AGENT.md 的设计规格一致：

| 入口 | 鉴权方式 | 状态 |
|---|---|---|
| `/api/v1/health` | 公开（探针/LB） | ✅ |
| `/api/v1/admin/auth/login|logout|session` | 公开（登录自身即凭据） | ✅ |
| `/api/v1/admin/*` | Session Cookie + CSRF Token | ✅ |
| `/api/v1/articles`, `/api/v1/search` | API Key + scope（readonly/readwrite） | ✅ |
| `/mcp` | API Key + scope | ✅ |

**关键变更**：

- `api.AuthMiddleware`（已完整实现但从未接线 → 接线到 `/api/v1`）
- `admin.SessionMiddleware`（同上）
- `admin.CSRFMiddleware`（从空实现 `c.Next()` → 同步器令牌模式，Token 在登录时生成、
  绑定 Session、GET 放行、写请求须 X-CSRF-Token 请求头）
- `RequireScope` 中间件：readonly 拒绝写入，readwrite 隐含 readonly
- API Key 从数据库按 SHA-256 哈希直接查库（`idx_api_keys_key_hash` 索引命中），
  替代此前只遍历内存切片且从未加载的实现
- MCP `Bearer ` 前缀剥离（此前把整个 Authorization 头当 Key 传入，必然哈希不匹配）
- 登录失败锁定（5 次 → 15 分钟，`ErrAccountLocked` → 429，此前规格写了但未实现）
- SameSite=Lax + HttpOnly Cookie 双重防护

**scope 验证**（实测确认）：readonly Key → GET 200、POST 403；readwrite Key → GET/POST 均 200；
伪造 Key → 401；缺 Bearer → 401；小写 bearer → 200。

#### API Key 管理

- `service.CreateKey`：生成 `ak_` 前缀的 24 字节随机明文，SHA-256 哈希存储，
  明文仅创建时一次性返回
- `service.ValidateKey`：按哈希查 `GetByHash`，带 `last_used_at` 异步节流回写
- `service.ScopeAllows`：readwrite 隐含 readonly
- CLI 子命令 `aiknow-server apikey create|list|revoke`，无头部署与救急可用
- 后台 API Key 管理页：创建/启用/禁用/删除，创建后显著提示立即保存明文

#### 前端 10 个页面

**新增 8 个页面模块**，此前只有 `index.html`（含 3 个占位 div）和 `login.html`：

| 页面 | 状态 |
|---|---|
| 仪表盘 | 统计卡片·状态分布·队列深度·服务健康·最近活动·失败列表 |
| 文章列表 | 筛选/搜索/排序/分页/行内操作/批量操作 |
| 文章编辑 | 正文编辑/AI 结果修正/状态面板/Chunk 预览/正文变更自动重新处理 |
| 文章录入 | 手动录入/URL 采集（含预览） |
| 检索测试 | 混合检索分阶段结果/耗时明细/关键词检索对照 |
| MCP 调试 | 参数表单/工具列表/scope 切换/原始请求响应/通过代理走 Session 免 Key |
| MCP 日志 | 筛选/全文检索/trace_id 查询/详情/清理过期 |
| 系统配置 | 24 项配置分 8 组展示/标量/JSON 编辑/变更历史/恢复默认 |
| API Key 管理 | 创建/启用/禁用/删除/明文一次性显示 |
| 登录 | 锁定提示/CSRF 下发 |

**前端架构**：
- 共享 API 客户端 `api.js`：CSRF Token 集中附加、401 统一跳登录、错误统一处理
- Alpine.js `Alpine.data()` 组件 + 客户端 History API 路由
- `index.html` 单一入口，模板全部内嵌，服务端无需 SSR
- app.js 排在 Alpine CDN 之前加载（`alpine:init` 注册组件 → Alpine 再启动）
- 新增 `x-cloak` 样式，避免模板渲染前的闪现

#### 16 个 admin handler TODO 全部消除

| 文件 | 变更 |
|---|---|
| `apikey.go` (5) | 全部实现，接 `authService.CreateKey/ListKeys/SetKeyActive/DeleteKey` |
| `setting.go` (6) | 全部实现，接 `SettingRepository` |
| `dashboard.go` (2) | 队列深度接 `pgqueue.Depth/DeadLetterCount`，健康检查实际 ping 库 |
| `log.go` | `DeleteExpired` 接 `mcpLogService.DeleteExpired`，支持 days 参数 |
| `search_debug.go` | `DebugKeywords` 接 `KeywordSearchService`，`NewSearchDebugHandler` 新增参数 |
| `mcp_debug.go` | 重写：走后端代理调 MCP 服务，鉴权重用 Session，scope 可切换 |
| `article.go` | 新增 Create/Preview，`BatchReprocess` 改为逐条汇总（此前 `_ =` 丢弃全部错误） |

#### 顺带修复的设计与实现缺陷

- **`setting.Value` 类型修正**：`map[string]interface{}` → `json.RawMessage`。
  迁移 005 播种的值是标量（`0.3`、`2000`），`map` 类型导致 `ListAll` 对每一行报
  `cannot unmarshal number into Go value of type map`，配置接口完全不可用。
  连带 `SettingHistory.OldValue/NewValue` 一并修正。

- **操作者列 UUID → TEXT**（新增迁移 009）：`api_keys.created_by`、`setting_history.changed_by`、
  `articles.deleted_by` 三列建表时为 UUID，但 AGENT.md 决策 #10 明确 v1 单管理员、
  凭据来自配置文件、**无用户表**——系统中根本不存在可填入的用户 UUID。
  实际后果是写入 `created_by` 时报 `invalid input syntax for type uuid`。

- **仓库层双重哈希陷阱**：`apikey_repo.Create` 在 `service.HashAPIKey` 已哈希的基础上
  又做了一次 `sha256`，导致写入值与校验值永不一致。改为仓库层只存不哈希，哈希入口唯一。

- **提示词持久化与管线接线**：
  - `prompt.GetPrompt` 从只返回硬编码一句话 → 优先取库中值，回退 L2 默认值
  - `prompt.UpdatePrompt` 从校验通过后 `return nil`（静默丢弃）→ 持久化到 settings 表
  - 流水线 `runLLM` 从硬编码提示词 → 通过 `PromptService` 取可修改的模板，
    否则后台改提示词对实际处理毫无影响

- **`DebugSearch` 桩实现**：共享 `Search` 的检索逻辑，把各阶段中间结果分别保留。
  此前两个方法拥有完全相同的检索代码，任意一方的逻辑变更都会导致调试页与
  生产检索行为出现分歧——调试页就此失去参考价值。

- **MCP 日志 `List` 查询**：完整的筛选/全文检索/分页实现。

- **`articles` 表已应用迁移 008**（`retry_count`/`last_error` 补列）

#### 新增测试

`auth_apikey_test.go`，11 项覆盖：ValidateKey 从仓库读取、哈希存储而非明文、
未知/停用 Key 拒绝、scope 校验、缺仓库报错、ScopeAllows、CSRF 会话绑定、
CSRF 跨会话不通用、登出后 CSRF 失效、登录锁定、成功登录清零失败计数。

#### 验证

- `go build` / `go vet` / `go test ./...` 全绿
- curl：51 条路由无冲突，匿名访问全 401，scope 逐级确认，CSRF 缺/错/对 三种情况
- Playwright 真实浏览器：8 个页面均正确渲染，零控制台错误、零失败请求
- 交互测：创建 API Key → 录入文章 → 列表出现 → 检索召回 → MCP 调试四阶段完整
- 进程资源：CPU 0%、RSS 26MB
- 测试数据已清理

#### 剩余 TODO（2 个，属 Phase 2）

`add_article_by_url` MCP 工具须接 Collector 入队；
`htmlclean` 须接 `bluemonday` 生产级 HTML 清洗。

## [v0.5.8] - 2026-09-09

### 修复页面无限重定向，并修复接口联调中暴露的 12 处缺陷

**起因**：服务启动后页面报 `ERR_TOO_MANY_REDIRECTS`。定位重定向根因后，
对全部接口做了一轮实际联调，又发现 11 处问题（含一处会耗尽内存的死循环）。

#### 1. 页面无限重定向（原始症状）

`internal/handler/web/handler.go` 的 SPA 回退用 `c.FileFromFS("/index.html", ...)`。
Gin 会把 `c.Request.URL.Path` 改写成目标路径后交给 `http.FileServer`，而
net/http 的 `serveFile` 对任何以 `/index.html` 结尾的路径**无条件 301 跳到 `./`**。
于是 `GET /` → NoRoute → 301 → `/` → NoRoute，无限循环。

改为 `serveHTML()`：直接从嵌入 FS 读取内容并 `c.Data()` 写出，绕开该重定向规则。

顺带发现前端预期的 `/ui/*` 路由**从未注册**（`/ui/login` 等全靠 NoRoute 兜底）。
补齐 `/`、`/ui`、`/ui/*path` 三条路由。gin 的 catch-all 不能与同层静态路由并存
（`/ui/login` + `/ui/*path` 会 panic），故登录页在 wildcard handler 内分流。

验证：`/`、`/ui`、`/ui/login`、`/ui/dashboard`、`/ui/articles`、`/ui/articles/new`、
`/ui/search`、`/ui/settings` 全部 `200 redirects=0`。

#### 2. `sql.ErrNoRows` 等值比较恒为假（9 处）

```go
if err == sql.ErrNoRows {      // 永远不成立
if errors.Is(err, sql.ErrNoRows) {   // 改为
```

本项目用 pgx 驱动，返回的是 `pgx.ErrNoRows`。它 wrap 了 `sql.ErrNoRows`，
所以 `errors.Is` 为真、`==` 为假（已实测确认）。后果是"查无结果"这一正常分支
被当作错误返回，例如创建文章时去重检查报
`check content duplicate: no rows in result set`。

涉及 `article_repo.go`（4）、`apikey_repo.go`（2）、`chunk_repo.go`、
`mcplog_repo.go`、`setting_repo.go`。`worker.go:121` 原本就用 `errors.Is`，未改。

#### 3. `articles` 表缺 `retry_count` / `last_error` 列

新增 `008_articles_retry_columns.sql`。模型与三处 SELECT 自始就在读这两列，
001 建表时漏掉，文章列表与详情接口报 `42703 column does not exist`。

#### 4. 列表分页恒为空

```go
append(args, offset, query.PageSize)    // SQL 是 LIMIT $N OFFSET $N+1
append(args, query.PageSize, offset)    // 改为
```

绑参顺序与占位符相反，首页 `offset=0` 使 `LIMIT 0`。表现为 `total` 正确
但 `articles` 恒为空数组。

#### 5. 去重命中返回 500

新增哨兵错误 `service.ErrDuplicateArticle`，Handler 映射为 **409 Conflict**。
去重命中属正常业务结果，原先用无类型 `fmt.Errorf`，调用方无从区分。

#### 6. AI 处理任务从未入队

全局搜索 `Send`/`Enqueue` **零调用**——文章创建后从不投递任务，worker 一直在
空队列上轮询，所有文章永久停留在 `pending`。

而 worker 的 `processMessage` 却是从消息载荷里取 title/content **再创建一次文章**，
与写入侧的同步创建重复，且必然撞上内容去重。这段代码因为没有入队方，从未执行过。

按 PRD"提交后立即返回，异步完成 AI 处理"的设计重新接线：

- 新增 `service.JobEnqueuer` 接口，由 `pgqueue.Queue` 实现
- 载荷只带 `article_id`，避免大文本进队列及与库中内容不一致
- `Create` 入队失败**不**回滚、不报错（文章已落库，报 500 会让调用方重试撞上 409），
  仅告警并停在 `pending`；`Reprocess` 入队失败**必须**报错
- `processMessage` 改为按 ID 读取文章后直接跑流水线
- main.go 中队列创建提前到服务初始化之前

#### 7. 文本切分死循环（会耗尽内存）

`SplitContent` 在末片不满 `chunkSize` 时无限循环。实测进程 **CPU 137%、RSS 4GB**。

130 字正文、`chunkSize=400`、`overlap=50`：首轮 `end` 截到 130，
`start = 130-50 = 80`；下一轮 `end` 又被截回 130，`start` 再算仍是 80。
`start` 恒定不变，每轮 append 一片。原兜底判断 `start >= end`（80 >= 130）
不成立，拦不住。

**几乎所有真实文章都会命中**（只要末片不满一个窗口）。修复：`end == totalLen` 即 break。

新增 `ai_pipeline_test.go` 覆盖 7 个边界（短于/等于/略超 chunkSize、末片不满、
整除无重叠、单字符、overlap 超限）。

#### 8. `range` 副本导致 chunk 字段写入丢失

```go
for i, chunk := range newChunks { chunk.ArticleID = ... }   // 改不到切片
for i := range newChunks { newChunks[i].ArticleID = ... }   // 改为
```

`ArticleID`、`ChunkIndex`、`Embedding` 三项赋值全部丢失，`article_id` 保持空串，
插库报 `invalid input syntax for type uuid: ""`。

（`chunk_repo.go:53` 形似但无问题：那里切片元素是指针。）

#### 9. `Update` 遗漏 4 个列

UPDATE 语句的 SET 列表缺 `indexed_at`、`embedding_model`、`embedding_dim`、
`last_error`。AI 流水线正是通过本方法保存这几项，导致处理完成后索引时间与
向量模型信息始终为空，失败原因也无处落库。

#### 10. 向量数据泄漏进检索响应

`VectorSearch` 的 SELECT 取了 `c.embedding` 并一路带进 JSON，每条结果多出
1024 维浮点数（约 4KB）。`omitempty` 对非空切片不生效，无法靠标签规避。

`SearchChunk.Embedding` 全局无消费者，故一并从 SQL 与模型中移除。
**响应体 22688 字节 → 1141 字节。**

#### 11. 任务转入死信后文章永久停留在 `processing`

流水线开始即把状态置为 `processing`，但只有 LLM 阶段失败会自行回写 `failed`；
切分、向量、写库阶段出错时无人收尾。新增 `markArticleFailed()`，队列转死信后
回写 `failed` 与 `last_error`（用独立的带超时 context，避免 worker 停止流程中
因 `w.ctx` 已取消而写不进去）。

#### 12. 前端 session 接口路径不一致

`app.js` 请求 `/api/v1/admin/session/me`（404），后端实际为
`/api/v1/admin/auth/session`。

#### 验证

`go build` / `go vet` / `go test ./...` 全部通过。端到端：录入立即返回
`pending`，约 2 秒后 `completed`，`indexed_at`、`embedding_model=mock`、
`embedding_dim=1024`、`summary` 均正确落库；向量检索与关键词检索均能召回；
进程 CPU 0.2%、RSS 24MB。测试数据已清理。

#### 未修复（需产品决策，详见 AGENT.md）

- **`/api/v1/*` 全部接口无鉴权**：匿名 POST 可写入、DELETE 可删除（实测确认）
- `admin` 包 34 个 handler 方法中仅 3 个（登录/登出/会话）注册了路由，
  仪表盘、文章编辑、系统配置、MCP 调试与日志、API Key 管理等页面所需接口均缺失
- API Key 从不从数据库加载，MCP 对任何 Key 均返回 401
- `SessionMiddleware` / `CSRFMiddleware` 从未接线，后者本身是空实现

## [v0.5.7] - 2026-09-08

### 修复 001 迁移建表顺序错误导致的启动失败

**症状**：全新数据库执行 `aiknow serve` 或 `aiknow migrate` 时首个迁移即失败：

```
Applying migration 1: 001_initial_schema.sql
迁移失败: execute migration 001_initial_schema.sql:
         ERROR: relation "categories" does not exist (SQLSTATE 42P01)
```

**根因**：`001_initial_schema.sql` 中 `articles` 表定义在文件最前，其
`category_id UUID REFERENCES categories(id)` 引用的 `categories` 表却排在
`articles` 之后。PostgreSQL 建表时外键的被引用表必须已存在，因此 `articles`
建表语句直接报错。

迁移执行器按 `;` 逐条提交语句（`internal/repository/migrations.go`），语句间
不做依赖重排，文件内的书写顺序即执行顺序。

**修复**：把 `categories`、`tags` 两张无外键依赖的基础表提到 `articles` 之前，
建表顺序改为：

```
categories → tags → articles → article_tags → chunks
```

表定义内容未作任何改动，仅调整顺序，无 schema 变更。

**影响范围**：仅影响从零初始化的数据库。此前已成功建库的环境
（`schema_migrations` 中已有 version 1）不受影响，无需重跑。

**验证**：全新库执行 `migrate`，001–007 七个迁移全部通过。

## [v0.5.6] - 2026-09-08

### 简化端口配置：移除监听地址与端口映射变量

**背景**：网络层访问控制由运维在安全组 / 防火墙层面负责，编排内再配一层监听地址
属重复且易给出错误的安全感；宿主机与容器用不同端口在本项目也无实际需求。

#### 移除的变量（4 个）

| 变量 | 处置 |
|---|---|
| `POSTGRES_BIND` | 移除，容器端口默认发布在所有网卡 |
| `APP_BIND` | 移除，同上 |
| `APP_PORT` | 移除，宿主机与容器同端口，由 `PORT` 统一控制 |
| `POSTGRES_PUBLISH_PORT` | 并回 `POSTGRES_PORT` |

端口映射由 `"${VAR}:${VAR}"` 形式改为两侧同步：

```yaml
postgres:  ports: ["${POSTGRES_PORT:-5432}:5432"]
app:       ports: ["${PORT:-8080}:${PORT:-8080}"]
```

`.env.example` 的变量数由 22 项减至 18 项，且**每一项都会被应用读取** ——
不再有"仅供编排插值"的分类。

#### `POSTGRES_PORT` 合并为单变量的前提

v0.5.5 曾因"一名两用"把它拆成两个变量（宿主机发布端口 vs 应用连库端口）。
本次合并回单变量，安全性由编排的 `environment` 覆盖保证：

```yaml
app:
  environment:
    POSTGRES_PORT: "5432"   # 覆盖 .env，容器网络内恒为 5432
```

`environment` 优先级高于 `env_file`，因此用户把 `.env` 中的 `POSTGRES_PORT`
改为 `15432` 只影响宿主机发布端口，容器内 app 仍连 `postgres:5432`，
不会重现 v0.5.5 修复的连错端口问题。

合并后语义反而更自洽：`POSTGRES_PORT` 统一表示"postgres 可达的端口" ——
compose 下是宿主机发布端口（供 `psql` / `make db-init` 直连），
直跑二进制时就是数据库实际端口，两者指向同一个数据库。

#### 文档

- `docs/DEPLOY.md`：「仅 Docker Compose 使用的变量」一节（表格已空）重写为
  「端口与监听地址」，明确"上表就是全部配置项"；删除「`PORT` 与 `APP_PORT`
  的区别」一节，改为说明 `POSTGRES_PORT` 在两种部署形态下的含义差异；
  编排要点两条同步更新
- `AGENT.md`：删除已不存在的 `APP_PORT` / `POSTGRES_PUBLISH_PORT` 描述，
  改为记录"端口不设映射变量"，并保留一条约束说明 —— `POSTGRES_PORT` 必须在
  `environment` 中覆盖为 5432，日后调整覆盖逻辑时需守住
- `.env.example`：`POSTGRES_PORT` 由注释提示提升为正文配置项并说明双重含义；
  末尾「仅 docker compose 使用」整段删除

**Go 代码无改动**：`POSTGRES_PORT` 本就在 `managedKeys` 中，应用侧语义未变。

**验证状态**：
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅
- compose YAML 解析 ✅（确认端口映射与 5 项覆盖符合预期）

**仍需实机验证**（本机无 Docker）：`environment` 对 `env_file` 中
`POSTGRES_PORT` 的覆盖是否如期生效。

---

## [v0.5.5] - 2026-09-08

### 数据库配置去重：DATABASE_URL 优先 + POSTGRES_* 分项兜底，compose 改用 env_file

**背景**：`.env` 中同时存在 `DATABASE_URL` 与 `POSTGRES_*` 两套数据库配置，
且 `docker-compose.yml` 的 app 服务逐项透传 14 个变量，冗长且易漏。

#### 配置层：数据库连接二选一

新增 `cfg.DatabaseURL() (string, ConfigSource, error)`：

- 优先 `DATABASE_URL`；为空时由 `POSTGRES_HOST/PORT/USER/PASSWORD/DB/SSLMODE`
  拼装
- 拼装使用 `net/url`，**密码经 URL 编码** —— 字符串拼接会让密码中的
  `@` `:` `/` `?` `#` 破坏 DSN 结构
- 两者皆无时返回明确错误，而非交给数据库驱动报难以定位的连接失败

分项新增默认值：`POSTGRES_HOST=postgres`（compose 服务名）、`PORT=5432`、
`USER=postgres`、`DB=aiknow`、`SSLMODE=disable`。因此 compose 场景只需给
`POSTGRES_PASSWORD` 一项即可拼出可用连接串。

`Validate()` 同步放宽：接受"仅分项"这一形态，此前硬要求 `DATABASE_URL`
会把 compose 配置误判为缺配置。

`config.yaml` 的 `database` 段同步支持分项（`host` / `port` / `user` /
`password` / `name` / `sslmode`）。

#### 编排：app 改用 env_file

- postgres 服务保持 `${VAR:-default}` 形式不变
- app 服务改为 `env_file: .env` 整体注入，`environment:` 仅保留 5 项必须在
  容器内覆盖的配置（`environment` 优先级高于 `env_file`，官方文档明确）：

  | 覆盖项 | 原因 |
  |---|---|
  | `DATABASE_URL: ""` | 屏蔽 `.env` 中面向宿主机的 DSN |
  | `POSTGRES_HOST: postgres` | 容器网络内按服务名访问 |
  | `POSTGRES_PORT: "5432"` | 容器网络内端口，与宿主机发布端口无关 |
  | `APP_ENV` | 本编排走 HTTP，默认 development |
  | `PORT` | 容器内监听端口 |

  app 服务的 `environment` 段由 14 项减至 5 项。

**关键处理**：`env_file: .env` 会把本地开发用的
`DATABASE_URL=...@127.0.0.1:5432/...` 原样带进容器，而容器内 `127.0.0.1` 是
容器自己，应用会连不上且报错难以定位。因此显式置空 `DATABASE_URL`，让容器内
一律走分项拼装 —— 这使得 `.env` 中两套配置可以并存，各自在对应部署形态下生效。

#### 修复：POSTGRES_PORT 一名两用

`POSTGRES_PORT` 原为宿主机发布端口；引入分项拼装后它同时成为应用连库的端口。
若用户为避免端口冲突将其改为 `15432`，容器会去连 `postgres:15432`（网络内实为
5432），连接失败。

- 宿主机发布端口更名为 `POSTGRES_PUBLISH_PORT`
- `POSTGRES_PORT` 专指数据库实际端口，容器内由编排固定为 `5432`

#### 配置文件

- `.env.example` 重写数据库段：改为「方式 1 `DATABASE_URL`（直跑二进制）/
  方式 2 `POSTGRES_*` 分项（compose）」二选一结构，说明两者可并存的原因；
  `POSTGRES_USER/PASSWORD/DB` 从「仅 compose 使用」区移入正文（它们现在
  既用于建库也用于拼装连接串，一份值两处用）；末尾仅保留 4 个真正只供编排
  插值的端口映射变量
- `config.yaml.example` 数据库段补充分项字段

#### 文档

- `docs/DEPLOY.md` 第三章：「应用读取的配置项」表补入 6 个 `POSTGRES_*` 分项；
  「仅 Docker Compose 使用的变量」表精简为 4 个端口映射变量并说明为何
  `POSTGRES_USER/PASSWORD/DB` 不在其列；「`DATABASE_URL` 与 `POSTGRES_*` 的关系」
  一节由「compose 拼装后注入」改写为「二选一 + 应用侧拼装 + compose 置空」
- `AGENT.md`：必填项改为「数据库连接二者之一」；补充 `POSTGRES_PORT` 与
  `POSTGRES_PUBLISH_PORT` 的区分及其由来，以及 `env_file` 与 `environment`
  的优先级关系

#### 测试

`internal/config/config_test.go` 新增 8 个用例：

- `DATABASE_URL` 优先于分项
- 分项完整拼装 / 仅给密码时套用默认值
- **密码含 `@:/?#` 时的转义**（校验解析回原值，且主机名与路径未被污染）
- 两者皆无时报错
- `Validate()` 接受仅分项形态
- YAML 分项解析 / `database.port` 为 0 时不覆盖默认值

**验证状态**：
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅（新增 8 个用例，config 包共 24 个）
- `gofmt` ✅（本次改动文件均合规）
- compose YAML 解析 ✅（确认 5 项覆盖与端口映射符合预期）

**行为变更**：`app` 服务改用 `env_file: .env` 后，`.env` 文件**必须存在**
（此前可全部通过 shell 环境变量提供）。`docs/DEPLOY.md` 第二章已要求
`cp .env.example .env`，正常流程不受影响。

**仍需实机验证**（本机无 Docker）：compose 下 `environment` 对 `env_file`
的覆盖行为、`DATABASE_URL` 置空后应用是否正确走分项拼装。

---

## [v0.5.4] - 2026-09-08

### 配置体系修复 + AI Provider 与管理员凭据接线 + 三个安全修复

**背景**：核对 `.env.example` 中是否存在无效变量时，发现配置加载层存在实质缺陷，
且多处配置项虽被校验却从未接线。

#### 修复：PORT 与 APP_ENV 无法被任何来源覆盖

`L1Config` 上写了 `env:"PORT"` / `env:"APP_ENV"` 标签，但**没有任何代码读取这些标签**
（无反射加载）。三个加载函数各自维护一份硬编码键列表，二者都不在其中；`loadYAML`
也未将其写入 `c.values`。结果 `Get()` 永远走到默认值分支——在 `.env` 里改 `PORT`
完全无效，端口恒为 8080。

- 三份硬编码列表收敛为单一 `managedKeys` 清单，三个来源共用
- 新增配置项只需在清单中登记一次，避免再次漂移

#### 修复：config.yaml 整层失效

`config.yaml.example` 使用嵌套结构（`database.url`、`server.port`），而解析器期望
扁平键（`database_url`）。按示例文件填写的配置读进去全是空值，**整个 YAML 配置层
形同不存在**。

- 按你的选择，改解析器支持嵌套：新增 `YAMLConfig` 结构 + `toFlat()` 映射
- `config.yaml.example` 重写，补上缺失的 `app.env` 段，并移除代码不消费的字段
  （连接池、HTTP 超时、模型名、登录锁定等），改为在文件末尾集中说明哪些项由代码固定
- 新增 `TestExampleYAMLMatchesParser` 守住示例文件与解析器的一致性

#### 修复：配置来源归因错误

`godotenv.Load()` 会把 `.env` 内容注入进程环境，此后 `os.Getenv` 无法区分值来自
`.env` 还是真实环境变量，导致来源被误标。

- `NewConfig()` 在加载 `.env` 前先快照真实环境变量，并改用 `godotenv.Read()`
  单独读取文件内容做归因

#### 接线：AI Provider

`main.go` 原先硬编码 `"mock"`，`.env` 中标着"必填"的 `QWEN_API_KEY` / `BAAI_API_KEY`
从未被使用。

- 三个 Provider 各自独立选择（`LLM_PROVIDER` / `EMBEDDING_PROVIDER` /
  `RERANKER_PROVIDER`），缺省 `mock`，无凭据也能启动
- 选择 `qwen` / `baai` 时对应 API Key 必填，缺失则启动时报错退出
- 顺带修复：三个 `factory.Create*` 的返回错误此前以 `_` 丢弃，provider 为 nil
  时要到首次调用才 panic

#### 接线 + 安全修复：管理员凭据

`auth.go` 的 `Login` **硬编码 `admin`/`admin`**，配置中的用户名与密码从未生效。

- 凭据改由 `AuthConfig` 注入；优先 `ADMIN_PASSWORD_HASH`，为空时才用明文
  `ADMIN_PASSWORD`（启动时在内存哈希）
- 改用 `bcrypt.CompareHashAndPassword` 校验；用户名用 `subtle.ConstantTimeCompare`，
  且无论用户名是否匹配都执行 bcrypt 比较，使失败路径耗时一致
- 启动时预校验哈希格式与明文长度（bcrypt 上限 72 字节，超长会被静默截断）
- 两者皆空时构造失败，不再以"无法登录"的状态启动
- 配置校验新增：生产环境禁止明文密码、`SESSION_SECRET` 长度 ≥32、
  `SESSION_LIFETIME` 格式合法

#### 安全修复：Session ID 可预测

`generateSessionID` 以 `time.Now().UnixNano()` 逐字节填充，输出可被预测，
攻击者能据此伪造他人 session。

- 改用 `crypto/rand` 生成 256 位随机 ID
- Session 新增过期时间（此前无过期概念），`GetSession` 校验并顺带清理过期条目

#### 安全修复：SessionMiddleware 不校验有效性

`internal/handler/admin/middleware.go` 仅检查 Cookie 是否存在便放行
（`// TODO: 验证 session 有效性`），任意伪造的 `session_id` 都能通过。

- 改为接收 `authService` 并实际调用 `GetSession` 校验
- 校验通过后将用户名写入 `gin.Context`，供后续 handler 获取操作者

> **该中间件目前仍未被任何路由注册**，另有 8 个 admin handler
> （dashboard/setting/apikey/log/prompt/search_debug/mcp_debug/article）
> 已定义但未注册路由。属功能缺口，不在本次范围，见下方"遗留问题"。

#### 修复：连接池超时单位错误

```go
config.MaxConnLifetime = 30 * 60  // 注释写"30 分钟"，实际 1.8µs
config.MaxConnIdleTime = 10 * 60  // 注释写"10 分钟"，实际 600ns
```

这两个字段是 `time.Duration`（纳秒），连接几乎每次取用都被判定过期并重建。
已改为 `30 * time.Minute` / `10 * time.Minute`。

#### Cookie 行为由配置驱动

- `Secure` 原硬编码 `true`，本地 HTTP 开发时浏览器不回传 Cookie，无法登录；
  现由 `APP_ENV` 决定（仅 production 启用）
- `MaxAge` 原硬编码 24h，现与 `SESSION_LIFETIME` 保持一致
- `Logout` 补充清理服务端 Session（此前只清 Cookie，Cookie 被复制后仍可继续使用）

#### 配置与编排

- `.env.example` 按**消费方**重写：`[A] 应用读取` / `[B] 仅 compose 使用` 两区，
  补上缺失的 `ADMIN_USERNAME`、`SESSION_LIFETIME`、三个 `*_PROVIDER`
- `docker-compose.yml`：`PORT` 原硬编码 `"8080"`（compose 下改 `.env` 无效），
  改为 `${PORT:-8080}`，端口映射容器侧同步；`APP_ENV` 默认由 `production` 改为
  `development`（本编排走 HTTP，production 会因 Cookie Secure 导致无法登录）；
  补齐 `SESSION_LIFETIME` 与 Provider 相关变量透传

#### 文档

- `docs/DEPLOY.md` 新增第三章「配置项参考」：优先级、应用读取项与 compose 专用项
  两张表、`PORT` 与 `APP_PORT` 的区别、`DATABASE_URL` 与 `POSTGRES_*` 的关系、
  `config.yaml` 用法、尚未接线的配置项、由代码固定不可配置的项；后续章节顺延重编号
- `AGENT.md`：「必填环境变量」改写为「配置约束」，区分无条件必填与条件必填，
  记录 `managedKeys` 登记要求与 `PORT`/`APP_PORT` 的区别；修正 L1 配置表中
  「dimension 可配」的错误描述（实为硬编码于 provider 包）；更新 godotenv
  优先级说明

#### 测试

- 新增 `internal/config/config_test.go`：16 个用例，覆盖 PORT/APP_ENV 回归、
  嵌套 YAML 解析、三来源优先级、默认值、时长解析、六项校验规则、示例文件一致性
- 新增 `internal/service/auth_test.go`：17 个用例，覆盖凭据接线回归、
  硬编码 admin/admin 已失效、session ID 随机性与长度、过期、登出、构造期校验
- 全部通过

**验证状态**：
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅（新增 33 个用例全部通过）
- `gofmt` ✅（本次改动的 8 个文件均合规；另有 13 个文件在 HEAD 即不合规，未纳入本次范围）
- compose YAML 解析 ✅

**遗留问题**：

1. **后台路由未接入鉴权**：`SessionMiddleware` 已修好但未被任何路由注册；
   8 个 admin handler 已实现却未注册路由。接入需先确定鉴权边界
   （哪些接口公开、哪些需登录），属功能缺口
2. **`CSRFMiddleware` 仍为占位**：未校验 Token，现阶段依赖 Cookie 的 SameSite
3. **`SESSION_SECRET` 仍未被消费**：只被校验（必填 + 长度）。Session ID 走密码学
   随机源并存于服务端，暂不需要签名密钥；保留必填是为 CSRF Token / Cookie 签名预留
4. **Session 存于进程内存**：重启即全部失效，多实例部署无法共享，水平扩容前
   须迁至 Redis 或数据库
5. **13 个文件不符合 gofmt**：HEAD 既有问题，可用 `make fmt` 单独提交修复

---

## [v0.5.3] - 2026-09-08

### 部署文件结构调整 + 文档改名为 DEPLOY.md 并补充运维内容

**背景**：`deploy/postgres/init/000_init.sql` 四层目录只装一个文件，属过度分层。

#### 目录结构

- `deploy/postgres/init/000_init.sql` → 根目录 `docker-compose-initdb.sql`，
  `deploy/` 目录随之移除
- `docker-compose.yml` 改为挂载单文件：
  `./docker-compose-initdb.sql:/docker-entrypoint-initdb.d/000_init.sql:ro`
- `.dockerignore`：移除已不存在的 `deploy/`，改为排除 `docker-compose-initdb.sql`
- `Makefile`：`db-init` 与 `migrate-up` 的路径注释同步更新

Dockerfile 与 docker-compose.yml 保持在根目录：`.env` 由 compose 从 project
directory 加载，移入子目录会导致读不到 `POSTGRES_PASSWORD` / `SESSION_SECRET`；
且多数 PaaS 与 CI 默认在仓库根查找这两个文件。

> 文件名以 `docker-compose` 开头，但它**同时用于本地开发**（`make db-init`），
> 是扩展创建的唯一来源。已在文件头注释、`AGENT.md` 与 `docs/DEPLOY.md` 中标注，
> 避免误解为仅服务 compose。

#### 文档改名并补充运维内容

`docs/DATABASE_SETUP.md` → `docs/DEPLOY.md`（标题改为「部署与运维指南」）。
原文档已覆盖 Dockerfile、compose、应用启动与运维，早已不止「数据库搭建」。

架构说明新增「初始化职责划分」表，明确扩展与表结构各自的唯一来源。

新增运维内容：

- **健康检查与监控**：健康端点、容器健康状态查询、关键观测指标 SQL
  （处理状态分布、队列深度、死信数量、最早任务等待时长、各表体积）
- **备份与恢复**：`pg_dump -Fc` 备份、恢复前必须先建扩展（转储含 paradedb /
  hnsw 索引的 CREATE INDEX，依赖扩展存在）、仅备份数据的做法
- **故障排查表**：8 类症状与排查方向
- **凭据轮换**：`SESSION_SECRET` / `ADMIN_PASSWORD_HASH` / API Key /
  `POSTGRES_PASSWORD` 各自的影响面与操作
- **升级 ParadeDB 的步骤**：6 步流程，含备份与索引语法变更的连带修改
- **生产部署补充建议**：TLS 前置、不暴露数据库端口、备份自动化、日志收集、
  资源限制、监控告警
- 队列运维补充租约机制说明（处理耗时可能超 5 分钟需调大 `LeaseDuration`）
- `chunks.deleted` 补充不一致时的手动修复语句
- 日志表清理补充 `VACUUM` 与 L2 配置 `mcp_log.retention_days`

#### 顺带修正的过时引用

- `docs/MILESTONES.md`：验收项 2 由「迁移 001–005」改为「001–007」；
  验收项 3 列的扩展改为 pgvector / pg_search（原列 pg_textsearch / zhparser /
  PGMQ 均已移除）；验收项 4 移除已被 paradedb 索引取代的 GIN 索引
- `cmd/aiknow-server/main.go`、`pkg/pgqueue/pgqueue_test.go`、
  `internal/repository/migrations/006_paradedb_search.sql`、`AGENT.md`：
  文档路径与初始化脚本路径

**验证状态**：
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅
- compose YAML 解析与挂载源文件存在性 ✅

**仍需实机验证**（本机无 Docker）：compose 首次启动能否正确执行
`docker-compose-initdb.sql`，以及第四章列出的三项待验证事项。

---

## [v0.5.2] - 2026-09-08

### 修复：表结构定义收敛为单一来源，修复迁移事务未提交

**背景**：v0.5.1 为方便开发期建表，在 `deploy/postgres/schema.sql` 放了一份
"合并版 DDL"。这属于同一份表结构存在两处定义，本次核查发现**两处已经漂移，
且 `schema.sql` 是坏的**——它是凭重构而非逐字合并写出来的，四张表都对不上 Go 代码：

| 表 | `schema.sql` 的偏差 | 后果 |
|---|---|---|
| `job_queue` | 丢失 `queue_name` 列 | `pkg/pgqueue` 用到该列 10 次，所有队列操作报错 |
| `job_archive` | 丢失 `queue_name`，多出 3 个 NOT NULL 列 | 归档 INSERT（`pgqueue.go:277`）失败 |
| `api_keys` | `is_active` 被改名为 `enabled`，丢失 `created_by` | `apikey_repo.go` 5 处查询报错 |
| `mcp_request_logs` | 整表结构不同（丢 `trace_id`/`tool_name`/`request_body`/`api_key_id`） | `mcplog_repo.go` 全部报错 |
| `settings` | 丢失 8 个默认配置项，凭空多出 `search.vector_top_k` | LLM / embedding 配置缺失 |
| 索引 | `idx_job_queue_poll` 被改名为 `idx_job_queue_dequeue` | 与 007 不一致 |

任何人用它初始化开发库，跑起来就是一片报错。

#### 表结构收敛为单一来源

- **删除 `deploy/postgres/schema.sql`**
- 表结构的唯一来源确定为 `internal/repository/migrations/`
- 新增 `migrate` 子命令：`aiknow-server migrate` 执行迁移后退出，
  供开发期初始化/重建与 CI 使用。它与 `serve` 共用 `runMigrations()`，
  **执行的是同一套代码**，从根上杜绝漂移

扩展创建是唯一例外，仍在迁移之外——迁移执行器会跳过一切扩展创建语句
（`migrations.go:75`），这是硬约束。

#### 扩展初始化脚本合并为单文件

- `deploy/postgres/000_init.sql` + `init/01-extensions.sql`（两文件 + 一层 `\ir` 间接）
  合并为 `deploy/postgres/init/000_init.sql`
- **同时修复一个部署期缺陷**：compose 仅挂载 `deploy/postgres/init` 目录，
  容器内 `\ir ../000_init.sql` 会解析到未挂载的父目录，**首次启动初始化必然失败**。
  合并为单文件后挂载路径自然对齐

#### 修复迁移事务从未提交（独立缺陷）

`cmd/aiknow-server/main.go` 中 `Begin` 后只有 `defer tx.Rollback(ctx)`，缺少 `Commit`。
且 `serve` 会阻塞在 HTTP 服务上，导致该事务在进程存活期间一直保持打开，
DDL 始终处于未提交状态——连接池内其他连接看不到任何新建的表。

- 抽出 `runMigrations(ctx, db)`：在单个事务内执行迁移并 `Commit`
- `serve` 与 `migrate` 共用该函数

#### Makefile

- `migrate-up`：由占位回显改为实际执行 `aiknow-server migrate`
- 新增 `db-init`：创建扩展
- 新增 `db-reset`：清空 `public` schema 后重新初始化扩展与表结构

#### 文档

- `docs/DATABASE_SETUP.md`：第一章改为「启动容器 → 创建扩展 → 配置 .env →
  执行迁移 → 启动应用」，重建数据库改用 `make db-reset`
- `AGENT.md`：明确「表结构的唯一来源就是 migrations 目录」，
  记录 schema.sql 的漂移教训；补充 `RunMigrations` 不负责提交、调用方必须 Commit

**验证状态**：
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅
- `--help` 含 `migrate` 子命令 ✅
- `migrate` 缺少 `DATABASE_URL` 时正确报错退出 ✅

---

## [v0.5.1] - 2026-09-08

### 开发环境优化：SQL 文件重组 + 文档重写

**背景**：v0.5.0 的部署文件面向测试/生产，本地开发仍需手动拼装初始化步骤。

#### SQL 文件重组

- 新增 `deploy/postgres/000_init.sql`：基础设施（扩展创建 + 可用性验证），
  用于本地开发初始化
- 新增 `deploy/postgres/schema.sql`：完整建表 DDL（合并 001/005/007 + 
  002 索引 + 006 deleted 列与触发器），用于本地开发快速重建
- 更新 `deploy/postgres/init/01-extensions.sql`：改为调用 `000_init.sql`，
  避免重复维护

**说明**：
- 生产环境仍使用 `internal/repository/migrations/` 中的迁移文件（已通过
  go:embed 打包进二进制）
- 000_init.sql / schema.sql 仅用于开发期快速重建数据库

#### 文档重写

`docs/DATABASE_SETUP.md` 完全重写，按使用场景拆分：

**第一章：本地开发环境（仅启动数据库）**
- 应用在宿主机以二进制运行，数据库在容器内
- 步骤：启动容器 → 执行 000_init.sql → 执行 schema.sql → 配置 .env → make run
- 补充"重建数据库"一节

**第二章：测试环境（Docker Compose 全栈）**
- 一键拉起数据库与应用
- 明确首次启动的四个阶段（拉镜像 → 构建 → 初始化 → 迁移）
- 补充常用操作（停止、进 psql、查看版本、重建）

**第三章：版本注意事项**（原"版本注意事项"节前置）

**第四章：待验证事项**（原散落在各处）
- UUID 作 key_field（含处置方案）
- jieba 召回效果
- KeywordSearch 的加权 SQL

**第五章：运维须知**（原"运维须知"）

**第六章：云托管的限制**（原末尾）

---

## [v0.5.0] - 2026-09-08

### ARCH-01：数据库架构调整 —— ParadeDB 替换 zhparser，数据表队列替换 PGMQ

**背景**：原方案需编译安装 3 个扩展（pg_textsearch、zhparser、pgmq），运维成本高。
zhparser 依赖 SCWS 词库同样需编译，并未比 ParadeDB 更省事；且内置 `ts_rank` 是
TF-IDF 变体，缺少文档长度归一化，实现不了 PRD 要求的 BM25。

**方案**：
1. 检索：ParadeDB（内置 pgvector + pg_search + pg_cron），BM25 与中文分词（jieba）由 pg_search 提供
2. 队列：pgmq 不在 ParadeDB 镜像内，改用数据表 + `FOR UPDATE SKIP LOCKED`

#### 数据库迁移

- `003_pg_textsearch_bm25.sql`、`004_pgmq.sql`：降级为空操作存根
  （已有开发库中已记录为"已应用"，原地改写不会重跑，新逻辑必须放新版本号）
- 新增 `006_paradedb_search.sql`：
  - 清理旧 zhparser / tsvector 对象
  - `chunks` 新增 `deleted` 冗余列 + 两个同步触发器（`trg_sync_chunks_deleted`、
    `trg_inherit_chunk_deleted`），使 BM25 过滤条件可下推进索引，避免 topK 与分页失真
  - 建立 `chunks_search_idx`、`articles_search_idx`（`USING paradedb` + `pdb.jieba`）
- 新增 `007_job_queue.sql`：`job_queue` / `job_archive` 表、出队与死信索引、
  `updated_at` 维护触发器

#### 队列实现（`pkg/pgqueue`）

内部完全重写，**公开 API 保持兼容**（`worker.go` 与 `main.go` 几乎无需改动）：

- 补齐 pgmq 原本代做的能力：可见性超时（租约）、指数退避重试、死信、归档
- `MsgID` 由 `string` 改为 `int64`（对应 `BIGSERIAL`）
- 新增 `Fail()`（判定重试或转死信）、`ReclaimExpired()`、`DeadLetterCount()`、
  `PurgeArchive()`、`SendWithMaxRetries()`、`StartReclaimer()` / `StopReclaimer()`
- 超时回收由应用层 Go 定时器执行，不使用 pg_cron
  （pg_cron 只能配置在 `postgres` 库且不可更改，跨库调度需引入额外部署依赖）

#### 检索实现

- `search_repo.go`：
  - `BM25Search` 改用 `|||` 算子 + `pdb.score(id)`，过滤 `deleted = false`
  - `KeywordSearch` 由空 TODO 实现为多字段加权检索
    （标签 > 分类 > 标题 > 摘要 > 正文，标签与分类走精确匹配）

#### 顺带修复的既有缺陷

- **向量索引失效**：`VectorSearch` 原用 `<->`（L2 距离），而 HNSW 索引建的是
  `vector_cosine_ops`，算子与 opclass 不匹配导致索引未被使用、退化为全表扫描。
  改用 `<=>`
- **Rerank 结果恒为空**：原以 chunk 的 UUID 与 Rerank 返回的数组下标比较，
  永不相等，精排结果被整体丢弃。提取 `applyRerank()` 按下标回取
- **RRF 名次互相覆盖**：原用同一个 map 记录两路名次，导致 `VectorRank` 与
  `BM25Rank` 恒相等；且未应用 `VectorWeight` / `BM25Weight`。改为分别记录并加权，
  名次由 0 起算修正为由 1 起算
- **`MaxRetries` 配置未生效**：原 `incrementRetryCount` 是空实现，重试从未发生。
  现由 `queue.Fail()` 落实
- **`splitSQL` 不识别美元引用**：`$$...$$` 内的 `;` 会被误切，导致 PL/pgSQL
  函数体无法执行（本次新增触发器即依赖此能力）。重写为支持美元引用（含带标签
  形式）、`$1` 占位符、`''` 转义引号、行注释与块注释

#### 测试

- 新增 `internal/repository/migrations_test.go`：`splitSQL` 5 个用例
- 新增 `pkg/pgqueue/pgqueue_test.go`：退避与配置默认值 5 个用例
- 新增 `internal/service/search_test.go`：RRF 融合与 rerank 映射 10 个用例
  （覆盖上述两个回归缺陷）
- 全部通过：`go test ./...` ✅

#### 部署

- 新增 `Dockerfile`：多阶段构建，静态二进制（embed 了静态资源与迁移脚本）
- 新增 `docker-compose.yml`：一键拉起 ParadeDB + 应用，健康检查确保扩展就绪后再启动应用
- 新增 `deploy/postgres/init/01-extensions.sql`：通过镜像的 `docker-entrypoint-initdb.d`
  机制自动创建扩展（仅首次初始化时运行）
- 新增 `.dockerignore`：控制构建上下文，排除文档与日志
- 更新 `Makefile`：修正构建路径（`./cmd/aiknow` → `./cmd/aiknow-server`）、
  `run` 目标补 `serve` 子命令，新增 `docker-build` / `up` / `down` / `logs` / `ps` 目标
- 更新 `.env.example`：补充 Docker Compose 所需的数据库与端口映射变量
- 修复 `cmd/aiknow-server/main.go`：`version` 由 `const` 改为 `var`，使 ldflags 注入生效；
  `--version` 输出格式改为 `version (commit hash)`

#### 服务层接线

- `SearchServiceImpl.SearchKeywords` 与 `KeywordSearchServiceImpl.SearchKeywords`
  由「未实现」桩接入仓库层实现
- 删除 `getQueryTokens()` 死代码（原用于调用 zhparser 分词，现由数据库侧 jieba 完成）

#### 文档

- 重写 `docs/DATABASE_SETUP.md`（原根目录版本内容已过时，扩展名 `pg_bm25`、
  `pg_jieba` 均有误，且称 pgmq 为镜像内置）
- 更新 `AGENT.md`：技术栈、迁移顺序、中文分词配置，新增
  「ParadeDB / pg_search 使用约束」「任务队列约束」「迁移执行器约束」三节
- 更新 `README.md` 技术栈

**验证状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过
- `go test ./...` ✅ 全部通过

**遗留问题**：

1. **待实机验证**（本机无 Docker，均无法执行）：
   - jieba 中文分词的实际召回效果
   - UUID 作 `key_field` 是否被 pg_search 接受（文档未明确背书；
     若报类型错误需改用 bigint 代理键，`docs/DATABASE_SETUP.md` 已记录处置方案）
   - 全套迁移在真实 ParadeDB 实例上的执行
   - `KeywordSearch` 的加权 SQL（含 CTE 与 `|||` 算子）尚未在真实实例上跑过
2. **`docs/DATABASE_SETUP.md` 原未跟踪版本丢失**：会话开始时该文件为 257 行未跟踪
   新文件，编写过程中丢失且从未进入 git，无法恢复。现版本基于 HEAD 中的根目录旧版
   （108 行）重写，并已按新架构全面更新。

---

## [v0.4.0-rc7] - 2026-09-07

### COL-2-01：URL 采集器实现（collector.go）

**对应任务**：COL-2-01

**变更**：
- 新增 `internal/service/collector.go`：URL 采集器实现
  - `Collector` 结构体：封装 HTTP 客户端、SSRF 检查器、HTML 清洗器
  - `CollectorConfig`：超时、重定向次数、最小/最大内容长度、User-Agent 配置
  - `DefaultCollectorConfig()`：默认配置（超时 10s、最多 3 次重定向、最小内容 200 字符、最大内容 100KB）
  - `NewCollector()`：构造函数，自动应用默认配置
  - `Fetch()`：采集 URL 内容，包含 SSRF 检查、HTTP 请求、内容限制读取、标题提取、HTML 清洗、最小长度校验
  - `extractContent()`：提取 `<title>` 标签作为标题，使用 `pkg/htmlclean` 清洗为纯文本
  - 重定向时自动进行 SSRF 检查
  - 返回 `CollectorResult`（含 `Title`、`Content`、`Truncated` 字段）

**编译状态**：
- `go build ./...` ✅ 通过

---

## [v0.4.0-rc6] - 2026-09-07

### DOC-7-01：项目文档（README / API / MCP）

**对应任务**：DOC-7-01

**变更**：
- 新增 `README.md`：项目介绍、特性列表、技术栈、快速开始、API 文档、部署方式、开发命令、项目结构
- 新增 `docs/MCP.md`：MCP 协议版本、端点、认证、4 个工具说明（3 readonly + 1 readwrite）、Claude Desktop 配置示例
- 更新 `docs/API.md`：补全认证方式、错误响应、健康检查、认证、文章 CRUD、检索接口文档

**状态**：文档齐全，可按文档一键部署。

---

## [v0.4.0-rc5] - 2026-09-07

### W6：提示词管理 Handler 与 Service（HND-6-04）

**对应任务**：HND-6-04

**变更**：
- 新增 `internal/service/prompt.go`：提示词服务
  - `GetPrompt`：从默认 map 获取提示词模板，key 不存在时返回错误
  - `UpdatePrompt`：校验模板必须包含 `{{.Content}}` 且含 "json" 字样（Qwen JSON 模式要求）
  - `TestPrompt`：返回渲染后的提示词（TODO：后续接入 LLM 调用）
  - 定义 `PromptResult` 结构体用于测试结果返回
- 新增 `internal/handler/admin/prompt.go`：提示词管理 Handler
  - `GetPrompt`：GET `/api/v1/admin/prompts/:key` 获取提示词
  - `UpdatePrompt`：PUT `/api/v1/admin/prompts/:key` 更新提示词
  - `TestPrompt`：POST `/api/v1/admin/prompts/test` 测试提示词渲染

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过

---

## [v0.4.0-rc3] - 2026-09-07

### W6：MCP 调试代理 Handler（HND-6-02）

**对应任务**：HND-6-02

**变更**：
- 新增 `internal/handler/admin/mcp_debug.go`：MCP 调试代理 Handler
  - `Invoke` 方法：支持 Authorization Header 和 Session Cookie 双鉴权
  - 读取请求体并解析 JSON，返回请求内容 + 占位响应
  - 预留 TODO 用于后续转发到 MCP 服务

**编译状态**：
- `go build ./...` ✅ 通过

---

## [v0.4.0-rc2] - 2026-09-07

### W6：Admin Handler 实现（HND-6-03/6-05/6-06）

**对应任务**：HND-6-03（setting）、HND-6-05（apikey）、HND-6-06（log）

**变更**：
- 新增 ： 系统配置 Handler
  - ：返回 L2 默认配置（从 model.DefaultL2Config）
  - ：按 key 获取单个配置（TODO）
  - ：更新配置（含参数校验）
  -  / ：恢复默认配置（TODO）
  - ：配置变更历史（TODO）
- 新增 ： MCP 日志 Handler
  - ：日志列表，含分页参数边界修正（Page/PageSize）
  - ：按 trace_id 获取单条日志
  - ：清理过期日志（TODO，预留 before 参数）
- 新增 ： API Key 管理 Handler
  -  /  /  /  / ：API Key CRUD（TODO）
- 修复 ：
  - 修正  包引用（重新导入 hashpwd 包）
  - 修正  多返回值问题（改为 ）
  - 修正  多返回值问题（改为 ）
  - 修正  参数类型（改为先开事务再传入）

**编译状态**：
-  ✅ 通过
-  ✅ 通过

---



## [v0.4.0-rc1] - 2026-09-07

### W5：MCP Server Handler（HND-5-01）

**对应任务**：HND-5-01

**变更**：
- 新增 `internal/handler/mcp/server.go`：MCP Server Handler 实现，支持 JSON-RPC 2.0 协议
  - `Handle` 方法：统一处理 MCP 请求，包含 API Key 鉴权、请求体读取、JSON-RPC 解析
  - `handleRequest`：路由分发，支持 `initialize`、`tools/list`、`tools/call` 三个方法
  - `handleInitialize`：处理 MCP 初始化请求，返回协议版本和服务端信息
  - `handleToolsList`：按 API Key scope 过滤返回可用工具列表
  - `handleToolsCall`：调用对应工具，错误时返回 JSON-RPC 标准错误格式
- 新增 `internal/model/search.go` 中 MCP JSON-RPC 类型：
  - `MCPJSONRPCRequest` / `MCPJSONRPCResponse`：JSON-RPC 2.0 请求/响应结构
  - `MCPError`：JSON-RPC 错误结构（code + message）
  - `MCPCallToolRequest`：tools/call 请求参数结构
- 更新 `internal/handler/web/handler.go`：`NewWebHandler` 支持可选注入 MCP handler，`RegisterRoutes` 注册 `/mcp` 的 GET/POST 路由
- 更新 `internal/service/interfaces.go`：已有 `MCPService` 接口定义（Initialize/ListTools/CallTool）

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过

---

## [v0.3.0-rc1] - 2026-09-07

### W3：检索服务层实现（SRC-3-01~3-03）

**对应任务**：SRC-3-01（检索仓库）、SRC-3-02（混合检索服务）、SRC-3-03（关键词检索服务）

**变更**：
- 新增 `internal/model/search.go`：补充 `SearchChunk`（检索分块结果）、`SearchTimings`（各阶段耗时）模型；`SearchResult` 重构为包含 Query/Results/Timings
- 新增 `internal/repository/search_repo.go`：`SearchRepositoryImpl` 完整实现，包含：
  - `VectorSearch`：基于 pgvector ` <-> ` 操作符的余弦距离检索
  - `BM25Search`：基于 PostgreSQL `ts_rank` + `plainto_tsquery` 的全文检索
  - `KeywordSearch`：占位实现，待接入 zhparser 分词
  - `GetArticleByID` / `GetCategories` / `GetTags`：委托现有仓库实现
- 新增 `internal/service/search.go`：`SearchServiceImpl` 完整混合检索实现（向量 + BM25 + RRF + Rerank），包含：
  - `Search` 方法：顺序执行 Embed → VectorSearch → BM25Search → RRF 融合 → Rerank 精排，记录各阶段耗时
  - `SearchWithConcurrent` 辅助方法：并发执行向量与 BM25 检索以提升性能
  - `rrfFuse` 函数：倒数排名融合（Reciprocal Rank Fusion），k=60
- 新增 `internal/service/search_keyword.go`：`KeywordSearchServiceImpl` 关键词检索服务实现（占位，待接入 zhparser）
- 更新 `internal/repository/interfaces.go`：新增 `SearchRepository` 接口（VectorSearch/BM25Search/KeywordSearch/GetArticleByID/GetCategories/GetTags）
- 更新 `internal/service/interfaces.go`：新增 `KeywordSearchService` 接口

**编译状态**：
- `go build ./internal/...` ✅ 通过
- `go vet ./internal/...` ✅ 通过

---

## [v0.2.0-rc1] - 2026-09-07

### W2：仓库层实现（REP-2-01~2-03）

**对应任务**：REP-2-01（文章仓库）、REP-2-02（Chunk 仓库）、REP-2-03（分类/标签仓库）

**变更**：
- 新增 `internal/repository/postgres.go`：`PostgresDB` 数据库封装（pgxpool + Tx 事务 + DB() 底层 sql.DB 适配 pgqueue）
- 新增 `internal/repository/article_repo.go`：`ArticleRepositoryImpl` 完整实现（Create/GetByID/List/Update/SoftDelete/Restore/CountByStatus/FindByURL/FindByContentHash/BulkUpdateStatus/BulkSoftDelete/GetArticleStatus/IncrementViewCount）
- 新增 `internal/repository/chunk_repo.go`：`ChunkRepositoryImpl` 完整实现（Create/BulkCreate/ListByArticle/DeleteByArticle/UpdateEmbedding/CountByArticle/GetTotalCount/GetByID），使用 `pgvector-go` 处理向量类型
- 新增 `internal/repository/category_repo.go`：`CategoryRepositoryImpl` 完整实现（Create/GetByID/GetByName/ListAll/GetOrCreateByName）
- 新增 `internal/repository/tag_repo.go`：`TagRepositoryImpl` 完整实现（Create/GetByID/GetByName/ListAll/BulkCreate/GetOrCreateByName/LinkTagsToArticle）
- 新增 `GenerateUUID()` 辅助函数（基于 `github.com/google/uuid`）
- 更新 `migrations/001_initial_schema.sql`：articles 表新增 `content_hash VARCHAR(64)` 字段及 `idx_articles_content_hash` 部分索引
- 更新 `go.mod`：新增 `github.com/google/uuid`、`github.com/pgvector/pgvector-go`、`github.com/jackc/pgx/v5/stdlib` 依赖

**编译状态**：
- `go build ./internal/repository/...` ✅ 通过
- `go vet ./internal/repository/...` ✅ 通过

---

## [v0.1.0-rc1] - 2026-09-07

### W1 基础框架初始化（里程碑 M1 达成）

**对应任务**：FND-1-01（项目骨架）、FND-1-03（数据库迁移）、FND-1-04（数据模型）、FND-1-05（L1 配置加载）、FND-1-06（接口契约）、AIP-1-01~1-03（AI Provider 接口）、PKG-1-01~1-03（公共库）、WEB-1-01（前端骨架）

**变更**：
- 新增 `go.mod`，Go 1.25 模块，依赖 pgx/v5、godotenv、yaml.v3、crypto
- 新增 `Makefile`，包含 build/run/test/vet/fmt/clean target
- 新增 `cmd/aiknow-server/main.go`，支持 hashpwd/version/help 子命令
- 新增 `cmd/aiknow-server/hashpwd/main.go`，密码哈希生成（占位实现）
- 新增 `migrations/001-005.sql`，完整数据库 Schema：articles、chunks、categories、tags、article_tags、settings、setting_history、api_keys、mcp_request_logs
- 新增 `internal/repository/migrations.go`，迁移执行器（embed + pgx 事务 + schema_migrations 版本管理）
- 新增 `internal/model/` 包：Article、Chunk、Category、Tag、Setting、APIKey、MCPRequestLog、SearchParams 等完整数据模型
- 新增 `internal/config/config.go`，L1 配置加载（环境变量 > .env > YAML > 默认值，来源日志，必填项校验）
- 新增 `internal/repository/interfaces.go`，定义 6 个 Repository 接口（Article/Chunk/Category/Tag/Setting/APIKey/MCPLog）
- 新增 `internal/service/interfaces.go`，定义 3 个 Service 接口（Article/Search/MCP）
- 新增 `internal/service/provider/` 包：LLM/Embedding/Reranker 三接口 + Qwen/BAAI/Mock 实现 + 工厂模式
- 新增 `pkg/ssrf/`：SSRF 防护库（内网 IP 拦截、协议白名单、端口限制），测试 7 项全部通过
- 新增 `pkg/htmlclean/`：HTML 清洗库（XSS 防护），测试 7 项全部通过
- 新增 `pkg/pgqueue/`：PGMQ 队列封装（send/pop/archive/深度查询），测试跳过（需 DB）
- 新增 `web/` 前端骨架：index.html、login.html、app.js（Alpine.js 路由）、main.css（Tailwind）

**编译状态**：
- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过
- `go test ./pkg/...` ✅ 通过（ssrf 7 项、htmlclean 7 项）
- `./aiknow-server --help` ✅ 正常
- `./aiknow-server version` ✅ 输出 0.1.0

**代码量**：27 个 Go 文件，2668 行代码

**里程碑 M1 验收状态**：
| # | 验收项 | 状态 |
|---|--------|------|
| 1 | go build 通过，./aiknow-server --help 显示子命令 | ✅ |
| 2 | 数据库迁移 001-005 文件创建完成 | ✅ |
| 3 | pgvector/pg_textsearch/zhparser/PGMQ 扩展 SQL 已就绪 | ✅ |
| 4 | HNSW/BM25/GIN 索引定义在迁移文件中 | ✅ |
| 5 | 环境变量 > .env > config.yaml > 默认值 优先级已实现 | ✅ |
| 6 | .env 不存在时静默跳过 | ✅ |
| 7 | 启动日志打印配置来源 | ✅ |
| 8 | 缺必填项启动失败报错 | ✅ |
| 9 | L2 配置缺键回落内置默认值 | ✅ |
| 10 | LLMProvider/EmbeddingProvider/RerankerProvider 三接口定义 | ✅ |
| 11 | Embedding mock 返回 1024 维 | ✅ |
| 12 | repository/service 接口契约已定义 | ✅ |
| 13 | 前端页面壳可导航 | ✅ |

**下一步**：
- FND-1-02：数据库连接池实现
- AIP-1-04：提示词引擎 + JSON 校验
- W2：仓库层实现（R1-R6）、服务层实现（S1-S4）

---

## [v3.5.2] - 2026-09-07

### 新增 AI Provider 层接口与基础实现（W1 关键任务）

**对应任务**：AIP-1-01~1-03

**变更**：
- 新增 `internal/service/provider/llm.go`：定义 `LLMProvider` 接口（Generate、GenerateJSON、Name）及 `MockLLM` 实现
- 新增 `internal/service/provider/embedding.go`：定义 `EmbeddingProvider` 接口（Embed、EmbedBatch、Name、Dimension）及 `MockEmbedding` 实现
- 新增 `internal/service/provider/reranker.go`：定义 `RerankerProvider` 接口（Rerank、Name）及 `MockReranker` 实现
- 新增 `internal/service/provider/factory.go`：`ProviderFactory` 工厂模式，按配置字符串创建对应 Provider
- 新增 `internal/service/provider/llm_qwen.go`：Qwen LLM Provider 实现，兼容 DashScope ChatGPT 格式 API
- 新增 `internal/service/provider/embedding_baai.go`：BAAI Embedding Provider 实现，兼容 DeepInfra API
- 新增 `internal/service/provider/reranker_baai.go`：BAAI Reranker Provider 实现，兼容 DeepInfra API

**Mock 实现**支持开发/测试无需真实 API Key，默认维度 1024。

---

## [v3.5.1] - 2026-09-07

### 新增公共库包（W1 PKG 任务）

**对应任务**：PKG-1-01（ssrf）、PKG-1-02（htmlclean）、PKG-1-03（pgqueue）

**变更**：
- 新增 `pkg/ssrf/ssrf.go`：SSRF 防护库，支持协议白名单、端口限制、私有 IP 段拦截（含 AWS metadata、RFC 1918 等），提供 `SafeError` 友好错误信息
- 新增 `pkg/ssrf/ssrf_test.go`：覆盖允许/拦截场景及辅助函数（IsBlocked、IsPrivateIPError、IsSSRFResult）
- 新增 `pkg/htmlclean/htmlclean.go`：HTML 清洗器，支持标签移除与常见实体解码（预留 bluemonday 扩展点）
- 新增 `pkg/htmlclean/htmlclean_test.go`：覆盖标签清洗与实体解码
- 新增 `pkg/pgqueue/pgqueue.go`：PGMQ 队列封装（Send/Pop/Archive/Delete/Depth/OldestMessageAge）
- 新增 `pkg/pgqueue/pgqueue_test.go`：跳过依赖数据库的集成测试

---

## [v3.5.0] - 2026-09-07

### W1 关键路径：数据模型与配置加载骨架

**对应任务**：FND-1-04（数据模型）、FND-1-05（L1 配置加载）、FND-1-06（repo/service 接口）、WEB-1-01（前端骨架）

**变更**：
- 新增 `internal/model/` 包，定义完整数据模型：Article、Chunk、Category、Tag、Setting、APIKey、MCPRequestLog 及检索/MCP 相关 DTO
- 新增 `internal/config/config.go`，实现三层配置加载（默认值 < .env < YAML < 环境变量），带来源追踪与必填项校验
- 新增 `internal/repository/interfaces.go`，定义 6 个 Repository 接口契约（Article/Chunk/Category/Tag/Setting/APIKey/MCPLog）
- 新增 `internal/service/interfaces.go`，定义 3 个 Service 接口契约（Article/Search/MCP）
- 新增 `cmd/aiknow-server/main.go`，子命令模式（hashpwd/version/help），占位实现
- 新增 `web/` 前端骨架：SPA 主页面、登录页、Alpine.js 路由、Tailwind CSS 样式
- 新增 `internal/model/search.go`，定义检索参数与结果结构（SearchParams、SearchResult 等占位类型）

**依赖**：`github.com/joho/godotenv`、`gopkg.in/yaml.v3`

---

## [v3.4.1] - 2026-09-07

### 移除关键词检索的「标签部分匹配」档

**评审意见**：标签本来就是一个词语，只能精确命中，不存在「部分匹配」。

**问题分析**：v3.4 设计的 6 档权重中，「标签部分匹配」（权重 4.0，`tags.name LIKE '%kw%'`）是错误的设计：

1. **假阳性且权重过高**。`Java` 会命中标签 `JavaScript`，`云` 会命中 `云原生`、`阿里云`、`云计算`、`私有云`——几乎等于不筛选。而 4.0 的权重比正文命中（1.0）高 4 倍，一个误命中就能把无关文章推到前面。

2. **与已有信号重复**。标签词通常也出现在标题或正文中，而标题/摘要/正文三档走的是 zhparser 分词的全文检索，已经覆盖了这类关联。部分匹配不提供新信息，只是给假阳性开了一个高权重通道。

**变更**：
- 权重档从 6 档减为 5 档，删除 `weight_tag_partial`
- 标签与分类**只做精确匹配**（`lower(name)` 相等）
- 删除 `pg_trgm` 扩展依赖和 `idx_tags_name_trgm` 索引

**最终权重表**：

| 命中位置 | 权重 | 匹配方式 |
|---------|------|---------|
| 标签精确 | 10.0 | `lower(tags.name)` 完全相等 |
| 分类精确 | 8.0 | `lower(categories.name)` 完全相等 |
| 标题 | 3.0 | 全文检索（zhparser） |
| 摘要 | 1.5 | 全文检索（zhparser） |
| 正文 | 1.0 | BM25 归一化后乘权重 |

**连带简化**：去掉 `pg_trgm` 后，关键词检索所需的额外索引减少到三个（标签/分类的 `lower(name)`、摘要 GIN、`article_tags.tag_id`），部署依赖也少了一个扩展。

**影响范围**：PRD 5.2.5 评分模型、4.2.3 索引、7.3.3 L2 配置表、5.3.5 调试接口、2.5.5 检索测试页

---

## [v3.4] - 2026-09-07

### MCP 工具扩展：2 个 → 4 个

| Tool | 功能 | scope | 状态 |
|------|------|-------|------|
| `search_knowledge` | 语义检索，返回 chunk 证据块 | readonly | 已有 |
| `find_articles_by_keywords` | 关键词检索，返回文章卡片 | readonly | **新增** |
| `get_article_detail` | 获取文章全文 | readonly | 已有 |
| `add_article_by_url` | 提交 URL 采集入库 | readwrite | **新增**（原 P2 提前至 v1） |

#### 1. 新增 `find_articles_by_keywords`

输入一个或多个关键词，返回最相关的 Top N 文章卡片（标题、摘要、分类、标签、链接）。

**与 `search_knowledge` 的分工**（两者返回粒度不同，非重复功能）：

| | `search_knowledge` | `find_articles_by_keywords` |
|---|---|---|
| 输入 | 自然语言问句 | 关键词数组 |
| 返回粒度 | Chunk 片段 | 整篇文章卡片 |
| 检索方式 | 向量 + BM25 + RRF + Rerank | 标签/分类精确匹配 + 全文检索加权 |
| 用途 | RAG 证据供给 | 文章发现、主题浏览 |

工具 `description` 中写明该区别，引导调用方大模型正确选择。

**评分模型**（标签/分类命中权重高于正文——用户给关键词时期望的是「打了这个标签的文章」，而非「正文恰好提到该词的文章」）：

| 命中位置 | 权重 | 匹配方式 |
|---------|------|---------|
| 标签精确匹配 | 10.0 | `tags.name` 完全相等（忽略大小写） |
| 分类精确匹配 | 8.0 | `categories.name` 完全相等（忽略大小写） |
| 标题命中 | 3.0 | 标题全文检索 |
| 摘要命中 | 1.5 | 摘要全文检索 |
| 正文命中 | 1.0 | BM25 分数归一化后乘权重 |

> 标签与分类**只做精确匹配**，不做 substring 匹配。理由见 v3.4.1 条目。

- 最终得分 = Σ(每个关键词的各项命中权重)
- 多关键词不做平均：命中数量本身是信号，命中 3 个关键词的文章自然高于命中 1 个的
- `match_mode`：`any`（默认）/ `all`（先过滤未命中全部关键词的文章再排序）
- 5 项权重放入 L2 配置 `keyword_search`，后台可调

**参数**：`keywords`（必填，1-10 个）、`limit`（默认 5，1-20）、`match_mode`（默认 `any`）

**返回**：`content` 为文章卡片 resource；`annotations.articles` 提供结构化字段，含 `matched` 说明每个关键词的命中方式（让大模型判断相关性强度——「标签精确匹配」比「正文提及」是更强信号）。

**无结果时** `isError` 为 `false`——检索无结果是正常业务情况，不是错误。

#### 2. 新增 `add_article_by_url`（v1 唯一写入工具）

原计划 P2，本次提前至 v1。行为与 `POST /api/v1/articles` 完全一致，复用同一套采集入库逻辑（SSRF 防护、去重、异步处理），立即返回不等待 AI 处理。

**参数**：`url`（必填）、`category_name`（可选）、`tags`（可选，与 AI 生成的合并去重）

**关键设计决策**：

- **去重命中不算错误**：返回 `isError: false` + 已存在提示。若返回 `true`，大模型会误以为操作失败而重试。
- **SSRF 拦截错误信息统一措辞**，不回显 DNS 解析结果或具体拦截原因。否则该工具会变成内网探测器——调用方可通过错误信息差异推断内网哪些 IP 存活。
- **写限流与 REST API 共享计数器**（按 API Key 维度，默认 20 req/min），避免绕过限流。

#### 3. `tools/list` 按 scope 过滤

`readonly` Key 调用 `tools/list` 只返回 3 个只读工具，看不到 `add_article_by_url`。

**理由**：让只读客户端根本看不到写工具，比让它看到却调用失败更合理——大模型不会反复尝试注定 403 的工具，也避免在对话中暴露「存在但你不能用」的能力。

调用层面仍然强制校验 scope，返回 403。

#### 4. 新增 REST 接口 `/api/v1/search/keywords`

与 MCP 的 `find_articles_by_keywords` 共用同一套检索逻辑，仅返回格式不同（REST 返回原始 JSON，MCP 包装为 `content` + `annotations`）。

#### 5. 新增数据库索引

```sql
-- 标签/分类大小写不敏感精确匹配
CREATE INDEX idx_tags_name_lower ON tags (lower(name));
CREATE INDEX idx_categories_name_lower ON categories (lower(name));

-- 摘要全文检索（标题已有索引）
CREATE INDEX idx_articles_summary_gin ON articles
USING gin (to_tsvector('chinese', coalesce(summary, '')));

-- 反向查标签
CREATE INDEX idx_article_tags_tag_id ON article_tags (tag_id);
```

#### 6. 新增 L2 配置组 `keyword_search`

| 字段 | 默认值 | 范围 |
|------|-------|------|
| `default_limit` | 5 | 1-20 |
| `weight_tag_exact` | 10.0 | 0-100 |
| `weight_category_exact` | 8.0 | 0-100 |
| `weight_title` | 3.0 | 0-100 |
| `weight_summary` | 1.5 | 0-100 |
| `weight_content` | 1.0 | 0-100 |
| `max_keywords` | 10 | 1-20 |

#### 7. 后台检索测试页增加关键词检索 Tab

- 多值关键词输入，`match_mode`、`limit` 可调
- 5 项权重可**临时覆盖**（不写入配置），用于对比不同权重下的排序
- 每篇文章展开显示得分构成：各关键词在各位置的命中与贡献分值
- 新增调试接口 `POST /api/v1/admin/search/keywords/debug`，支持 `weights_override`

> `contribution` 与 `weight` 可能不等：正文命中的贡献值是 BM25 归一化后乘权重，其余位置命中即贡献满额。

#### 8. 项目结构

新增 `internal/service/search_keyword.go`。

#### 9. 验收标准

新增 2 组：
- 10.5 关键词检索验收（12 条）
- 10.6 MCP 写入工具验收（9 条）

10.4 MCP 验收扩充至 11 条（覆盖 4 个工具 + scope 过滤），后续章节编号顺延至 10.12。

### 工期

保持 7 周。关键词检索（Phase 3）和 MCP 写入工具（Phase 5）预计各增 1.5-2 天，两阶段本就相对宽松。若延期，优先将 `match_mode=all` 和 `matched` 字段降为 P1。

---

## [v3.3] - 2026-09-07

### 需求优化（评审意见处理）

#### 1. AI 处理合并为单次 LLM 调用

**问题**：原方案摘要、分类、标签分三次调用 LLM，同一篇正文被输入三次，token 成本和处理耗时都接近三倍。

**变更**：
- 合并为一次调用，用 `response_format: {"type": "json_object"}` 约束返回结构
- 三个提示词配置项 `prompt.summary` / `prompt.category` / `prompt.tags` 合并为 `prompt.analyze`
- 返回 JSON 含 `summary`、`category`、`tags` 三个字段

**收益**：输入 token 降至约 1/3；三项结果出自同一次推理，一致性更好。

**Qwen JSON 模式注意事项**：使用 `json_object` 时提示词中必须出现 "json" 字样，否则 API 报错。模板保存时校验这一点。

**结果校验分级**（只有结构性错误才重试，避免因内容越界重跑整次调用）：

| 校验项 | 规则 | 失败处理 |
|--------|------|---------|
| JSON 可解析 | 能反序列化 | 重试 |
| 字段完整 | 三字段均存在 | 重试 |
| 类型正确 | summary 为串，tags 为串数组 | 重试 |
| 摘要长度 | 50-500 字 | 截断，不重试 |
| 分类有效 | 在候选列表内或「未分类」 | 置「未分类」，不重试 |
| 标签数量 | 1-15 个 | 截取前 10，不重试 |
| 标签长度 | 单个 ≤ 20 字 | 丢弃超长项，不重试 |

**影响范围**：`internal/service/llm.go`、`prompt.go`；`settings` 表配置键；工作流程图

#### 2. 移除 admins 表与角色体系

**问题**：v3.2 设计了 `admins` 表（含 role、failed_attempts、locked_until），却没有用户管理和密码修改页面，表的存在意义不成立。MVP 阶段只有单一管理员，`admin` / `superadmin` 的角色区分也是空转。

**变更**：
- 删除 `admins` 表
- 管理员账号改由 L1 配置提供：`ADMIN_USERNAME` + `ADMIN_PASSWORD_HASH`
- 登录失败锁定状态改为内存维护
- 后台 API 移除所有 `superadmin` 权限标注，API Key 管理页不再做角色判断
- 新增 `aiknow-server hashpwd` 子命令用于生成 bcrypt 哈希

**密码配置策略**：
- 生产环境只提供 `ADMIN_PASSWORD_HASH`
- `ADMIN_PASSWORD`（明文）为开发便利项，启动时在内存哈希，不落盘不写日志
- 两者都缺失时**启动失败并报错**，不再生成随机密码——随机密码只打印一次的做法在容器重启场景下容易丢失

**留待 v2**：多用户、角色区分、密码自助修改。

**影响范围**：删除 `internal/repository/admin_repo.go`；`internal/service/auth.go` 改为读配置

#### 3. audit_logs 替换为 mcp_request_logs

**问题**：通用审计日志在单管理员场景下信息量有限，而真正需要回溯的是 MCP 调用情况（排查「Claude 那边为什么没搜到」）。

**变更**：删除 `audit_logs` 表，新增 `mcp_request_logs` 表。

**新表核心字段**：
| 字段 | 用途 |
|------|------|
| `trace_id` | 优先取请求头 `X-Trace-Id`（不强制），未传则服务端生成 UUID；响应头回写，便于关联调用方日志 |
| `request_body` / `response_body` | 完整 JSON-RPC，支持一键重放 |
| `result_summary` | 检索类调用的命中 chunk 与分数，回答「为什么这条没被搜到」 |
| `is_error` | 覆盖 JSON-RPC 层 `isError=true` 和 HTTP 层非 2xx |
| `elapsed_ms` | 耗时 |

**新增页面** `/ui/mcp-logs`（替代原 `/ui/logs`）：
- 按时间/方法/工具名/调用方/错误状态/耗时区间筛选
- 按请求参数内容全文检索（GIN 索引）
- trace_id 精确查询
- 一键重放：跳转 MCP 调试页并回填参数
- 检索结果回溯

**写入方式**：响应返回后异步写入，不阻塞 MCP 主链路；写入失败仅记服务端日志。

**保留策略**：默认 30 天，`pg_cron` 每日清理，天数由 `mcp_log.retention_days` 配置。

**删除追溯的补偿方案**：换掉通用审计日志后，「谁删了这篇文章」会失去记录。因此在 `articles` 表补 `deleted_at` / `deleted_by` 两个字段——本来就是软删除，成本极低，避免删除操作完全无迹可循。所有列表查询默认附加 `WHERE deleted_at IS NULL`，并加部分索引 `idx_articles_not_deleted`。

**影响范围**：新增 `internal/service/mcplog.go`、`repository/mcplog_repo.go`、`model/mcplog.go`、`web/js/pages/mcp-logs.js`；删除 audit 相关文件

#### 4. 配置加载优先级明确

**变更**：L1 配置来源改为四级，先命中者生效：

```
1. 环境变量        （最高，适合容器/CI 注入）
2. .env 文件       （本地开发，不提交版本库）
3. config.yaml     （默认值与非敏感配置，可提交）
4. 代码内置默认值   （兜底）
```

**实现要点**：
- 用 `godotenv` 加载 `.env`，其默认行为是**不覆盖已存在的环境变量**，与上述优先级天然一致
- `.env` 不存在时静默跳过，不报错（生产通常只用环境变量）
- `.env` 写入 `.gitignore`，仓库只提供 `.env.example`
- 启动后打印各关键配置的**来源**（如 `DATABASE_URL from: env`），但不打印值

**新增文件**：`.env.example`、`.gitignore`

**新增环境变量**：`ADMIN_PASSWORD_HASH`（必填）、`ENV_FILE`（可选，默认 `./.env`）

**影响范围**：`internal/config/config.go`

### 连带调整

- `settings` 表移除 `updated_by` 字段，`setting_history` 移除 `changed_by`（单管理员下无意义）
- `api_keys` 表移除 `created_by` 字段
- 新增配置项 `mcp_log.retention_days`（默认 30，范围 1-365）
- 后台页面从 10 个调整为 9 个（移除审计日志页，新增 MCP 请求日志页）
- 迁移文件重命名：`005_admin_settings.sql` → `005_settings_apikeys_mcplogs.sql`
- 验收标准新增 10.8（MCP 请求日志，11 条）、10.10（配置加载，7 条），10.2 补充 4 条 AI 处理项
- 修正架构图中遗留的「768 维」

### 路线图

保持 7 周不变。v3.3 移除管理员管理相关工作量，新增 MCP 日志页，两者大致抵消。

---

## [v3.2] - 2026-09-07

### 需求完善

#### 1. Web 管理后台功能细化（新增 PRD 2.5 节）

明确后台共 10 个页面，采用左侧菜单 + 右侧内容区的单页应用布局，使用 Session Cookie 鉴权（区别于对外 API 的 API Key 鉴权）。

| 页面 | 路径 | 核心内容 |
|------|------|---------|
| 登录页 | `/ui/login` | 用户名密码登录，失败次数限制 |
| 仪表盘 | `/ui/dashboard` | 统计卡片、处理状态分布、队列状态、最近活动、服务健康 |
| 文章列表 | `/ui/articles` | 筛选、搜索、排序、行内操作、批量操作 |
| 文章编辑 | `/ui/articles/:id` | 正文编辑、AI 结果修正、处理状态面板、Chunk 预览 |
| 文章录入 | `/ui/articles/new` | 手动录入 / URL 采集 |
| 检索测试 | `/ui/search` | 分阶段结果、耗时明细、分词结果 |
| MCP 调试 | `/ui/mcp-debug` | 参数输入、原始请求响应、SSE、鉴权切换 |
| 系统配置 | `/ui/settings` | 模型参数、提示词、检索参数、采集参数 |
| API Key 管理 | `/ui/settings/api-keys` | 创建、禁用、删除（仅 superadmin） |
| 审计日志 | `/ui/logs` | 查询与筛选 |

**关键行为约定**：
- 修改正文保存 → `processing_status` 重置为 `pending` 并重新入队
- 仅修改 AI 结果（摘要/分类/标签）→ 不触发重新处理

#### 2. MCP 调试页面（新增）

新增 `/ui/mcp-debug` 页面，无需依赖 Claude Desktop 等外部客户端即可验证 MCP 接口。

**核心能力**：
- 按 `inputSchema` 动态渲染参数表单，支持表单/JSON 双模式
- 展示完整 JSON-RPC 请求体与原始响应
- `content` 中 resource 类型渲染为可跳转卡片
- 支持 `initialize`、`tools/list`、`tools/call` 三种操作
- 支持 SSE 模式，逐条追加显示事件流
- 可切换用 Session 或指定 API Key 调用（验证 scope 与 401/403 行为）
- 展示 `MCP-Protocol-Version`、`Mcp-Session-Id` 响应头

**设计决策**：页面通过后端 `/api/v1/admin/mcp/invoke` 代理接口转发，而非浏览器直连 `/mcp`。
- **原因 1**：避免在前端暴露 API Key
- **原因 2**：绕开浏览器对 MCP 端点 `Origin` 校验的限制

#### 3. 检索测试页面（新增）

新增 `/ui/search` 页面，用于调优 RRF 权重和 `ef_search`。

- 三栏展示向量召回、BM25 召回、RRF 融合候选
- 精排结果标注每条在 RRF 阶段的原始排名变化
- 各阶段耗时明细（embedding / vector_recall / bm25_recall / rrf / rerank / context_fill）
- 展示 zhparser 对 Query 的分词结果，便于排查中文检索问题

#### 4. 配置分层重构（重要变更）

将原本集中在 `config.yaml` 的参数按性质拆为三层：

| 层级 | 存储位置 | 内容 | 变更方式 |
|------|---------|------|---------|
| **L1 启动配置** | 环境变量 / `config.yaml` | 连接串、密钥、端口、连接池、provider、base_url | 改后需重启 |
| **L2 业务配置** | 数据库 `settings` 表 | 模型参数、提示词、检索参数、采集参数 | 后台修改，热生效 |
| **L3 凭据数据** | `api_keys`、`admins` 表 | API Key 哈希、管理员账号 | 后台管理 |

**分层原则**：
- 密钥类只走环境变量，不落到 `settings` 表或后台页面，避免明文凭据在数据库和 UI 中扩散
- 模型名、温度、提示词属于业务调优范畴，需频繁试错，放数据库
- `provider`、`base_url` 归 L1：切换服务商属部署级变更，通常伴随密钥更换
- `ai.embedding.dimension` 固定 1024 且归 L1：与 `chunks.embedding VECTOR(1024)` 表结构强绑定

**热生效机制**：
```
启动：环境变量 > config.yaml > 内置默认值 → L1（进程内不可变）
     settings 表 → 内存 ConfigCache
运行：后台保存 → 写表 + 写 history → 刷新 ConfigCache
     多实例：PostgreSQL LISTEN/NOTIFY 广播，各实例重载
```

`settings` 表缺失的键回落到内置默认值，首次部署无需灌数据；「恢复默认」即删除对应行。

#### 5. 提示词配置化

三类提示词（摘要、分类、标签）移至后台可编辑，使用 Go `text/template` 语法。

**可用变量**：
- `{{.Title}}` — 文章标题（全部提示词）
- `{{.Content}}` — 纯文本内容，超长截断至前 8000 字（全部提示词）
- `{{.Categories}}` — 当前全部分类名，逗号分隔（仅分类提示词）

**校验规则**：保存时校验模板可编译，且包含必需变量（缺少 `{{.Content}}` 拒绝保存）。

**试跑功能**：可选一篇已有文章，用未保存的模板调用 LLM，展示渲染后 Prompt 与模型返回，不写入文章数据。

#### 6. 新增数据表

| 表名 | 用途 |
|------|------|
| `settings` | L2 业务配置，key-value + JSONB |
| `setting_history` | 配置变更记录（操作者、时间、旧值、新值） |
| `api_keys` | API Key 管理，从 config.yaml 迁移至数据库 |
| `admins` | 管理员账号，含锁定状态与角色 |
| `audit_logs` | 审计日志 |

**API Key 存储决策**：仅存 SHA-256 哈希 + 前缀（用于列表展示），不存明文。
- 选 SHA-256 而非 bcrypt 的原因：API Key 是高熵随机串，不存在字典攻击风险；且每次请求都要验证，bcrypt 计算开销不可接受

对应新增迁移文件 `migrations/005_admin_settings.sql`。

#### 7. 新增后台管理 API（PRD 5.3 节）

前缀 `/api/v1/admin`，使用 Session Cookie 鉴权（不接受 API Key），要求 CSRF Token。共 19 个接口，其中关键接口：

- `POST /api/v1/admin/mcp/invoke` — MCP 调试代理，返回体同时含 `request` 和 `response`，便于复制到其他客户端复现
- `POST /api/v1/admin/search/debug` — 检索调试，返回 `query_tokens`、`stages`、`timings_ms`
- `PUT /api/v1/admin/settings/:key` — 配置更新，校验失败返回 400 并指明具体字段
- `POST /api/v1/admin/prompts/test` — 提示词试跑

#### 8. 项目结构调整

- 新增 `internal/handler/admin/` 包（10 个文件），与 `handler/api/` 分离，因两者鉴权方式不同
- 新增 `internal/service/` 下 `prompt.go`、`setting.go`、`auth.go`、`audit.go`
- 新增 `internal/repository/` 下 `setting_repo.go`、`apikey_repo.go`、`admin_repo.go`、`audit_repo.go`
- `internal/config/` 拆为 `config.go`（L1）、`defaults.go`（L2 默认值）、`cache.go`（缓存 + 热重载）
- `web/js/pages/` 按页面拆分为 8 个 JS 文件

#### 9. 环境变量调整

新增必填项 `SESSION_SECRET`（后台 Session 签名密钥，32 字节以上随机串）。

新增可选项 `ADMIN_USERNAME`、`CONFIG_PATH`。

**首次启动逻辑**：`admins` 表为空时用 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 创建 superadmin；未设 `ADMIN_PASSWORD` 则生成随机密码打印到启动日志（仅一次）。

#### 10. 验收标准补充

新增 4 组共 39 条验收项：
- 10.6 后台页面验收（11 条）
- 10.7 MCP 调试页面验收（10 条）
- 10.8 系统配置验收（9 条）
- 10.5 安全验收补充 3 条（API Key 哈希存储、401、403）

#### 11. 开发路线图调整：6 周 → 7 周

原 Phase 4 将「Reranker + 完整 Web 后台」压在同一周，细化后后台含 10 个页面（其中 MCP 调试、检索调试、系统配置三个交互较重），拆分为：
- **Phase 4**：精排 + 后台基础页（登录/仪表盘/文章列表/文章编辑）
- **Phase 6**：调试与配置页（检索测试/MCP 调试/系统配置/API Key/审计日志）

### 其他修正

- 修正术语表中遗留的「768 维」为「1024 维」（v3.1 遗漏项）

---

## [v3.1] - 2026-09-07

### 技术方案调整

#### 1. 向量维度调整
- **变更**：向量维度从 768 维调整为 1024 维
- **原因**：采用更大的模型 `bge-large-zh-v1.5` 以提升检索准确率
- **影响范围**：
  - `chunks.embedding` 字段：`VECTOR(768)` → `VECTOR(1024)`
  - `articles.embedding_dim` 默认值：768 → 1024
  - 配置文件中的模型：`bge-base-zh-v1.5` → `bge-large-zh-v1.5`

#### 2. API 接口规范明确
- **变更**：明确文章录入时分类和标签的创建逻辑
- **新增规范**：
  - `category_id` 和 `category_name` 二选一，优先使用 `category_id`
  - 分类通过 `category_name` 指定但不存在时，自动创建新分类
  - 标签不存在时自动创建，重复标签自动去重
- **影响范围**：`POST /api/v1/articles` 接口实现

#### 3. RRF 参数补充
- **新增配置参数**：
  - `rrf_k: 60`（RRF 倒数排名融合参数，标准值）
  - `vector_weight: 0.5`（向量检索权重）
  - `bm25_weight: 0.5`（BM25 检索权重）
- **影响范围**：
  - `POST /api/v1/search` 接口增加 `rrf_k` 参数
  - `config.yaml` 增加 RRF 相关配置
  - 检索服务实现需支持可配置的权重

#### 4. 中文分词配置完善
- **变更**：扩展中文分词词性映射
- **原配置**：`n,v,a,i,e,l`（名词、动词、形容词、成语、叹词、习用语）
- **新配置**：`n,v,a,i,e,l,j,r,d,m,q,t,z`
  - 新增：`j`(简称)、`r`(代词)、`d`(副词)、`m`(数词)、`q`(量词)、`t`(时间词)、`z`(状态词)
- **SQL 变更**：
  ```sql
  ALTER TEXT SEARCH CONFIGURATION chinese 
  ADD MAPPING FOR n,v,a,i,e,l,j,r,d,m,q,t,z WITH simple;
  ```
- **影响范围**：`migrations/003_pg_textsearch_bm25.sql`

#### 5. MCP 返回格式优化
- **变更**：`search_knowledge` 工具返回格式改为结构化 resource
- **原格式**：拼接字符串格式
  ```json
  {
    "content": [
      {"type": "text", "text": "找到以下相关文档片段：\n\n1. [标题]..."}
    ]
  }
  ```
- **新格式**：结构化 resource 格式
  ```json
  {
    "content": [
      {"type": "text", "text": "找到 2 个相关文档片段"},
      {
        "type": "resource",
        "resource": {
          "uri": "aiknow://article/uuid/chunk/index",
          "name": "标题 - 第X节",
          "description": "内容片段...",
          "mimeType": "text/plain"
        }
      }
    ]
  }
  ```
- **优势**：
  - 结构化数据便于大模型解析
  - 支持 MCP 资源引用协议
  - 便于追踪和调试
- **影响范围**：`internal/handler/mcp/server.go` 中的 `search_knowledge` 实现

#### 6. 配置参数补充
- **新增附录 C 参数**：
  - `RRF k: 60`（倒数排名融合参数）
  - `向量权重: 0.5`
  - `BM25 权重: 0.5`
  - `向量维度: 1024`
  - `中文分词词性: n,v,a,i,e,l,j,r,d,m,q,t,z`（含词性说明）

### 评审意见处理
- ✅ 确认 `pg_textsearch` 为 Timescale 官方扩展，已被主流云厂商支持
- ✅ 采纳向量维度调整建议（1024 维）
- ✅ 采纳 API 接口规范明确建议
- ✅ 采纳 RRF 参数补充建议
- ✅ 采纳中文分词配置完善建议
- ✅ 采纳 MCP 返回格式优化建议
- ⏭️ 性能指标、部署方案、开发路线图保持原方案

---

## [v3.0] - 2026-09-07

### 初始评审版本
- 统一命名为 AiKnow
- 修正数据模型（category_id + article_tags）
- 明确 pg_textsearch + zhparser
- 缩减 v1 范围
- 补充 MCP 规范细节
- 补充安全需求
- 调整性能指标

---

## [v0.1.0-rc1] - 2026-09-07

### W1-W6 全部开发完成

**里程碑状态**：
- M1 (W1): ✅ 基础框架 - 已关闭
- M2 (W2): ✅ 采集+AI处理 - 已关闭
- M3 (W3): ✅ 检索核心 - 已关闭
- M4 (W4): ✅ 后台基础页 - 已关闭
- M5 (W5): ✅ MCP 服务 - 已关闭
- M6 (W6): ✅ 调试与配置 - 已关闭
- M7 (W7): 🟡 测试与发布 - 进行中

**代码统计**：
- Go 文件：67 个
- 代码行数：7,215 行
- Git 提交：17 次
- 测试套件：4 个（全部通过）

**已完成功能**：
- ✅ 文章 CRUD + 手动/URL 采集（含 SSRF 防护）
- ✅ AI 异步处理流水线（LLM → Chunk → Embedding）
- ✅ 混合检索（向量 + BM25 + RRF + Rerank）
- ✅ 关键词加权检索
- ✅ MCP 4 工具实现
- ✅ API Key 鉴权 + 限流
- ✅ Session 认证 + CSRF
- ✅ 仪表盘 + 队列监控
- ✅ 检索调试 + MCP 调试代理
- ✅ 系统配置 + 提示词管理
- ✅ L2 配置缓存热重载
- ✅ SSRF/XSS 安全防护
- ✅ Web 管理后台（10 个完整页面）

**文档完成**：
- ✅ README.md / API.md / MCP.md
- ✅ CHANGELOG.md / TICKETS_BY_WEEK.md
- ✅ MILESTONES.md / DEVELOPMENT_PLAN.md
- ✅ .env.example / DATABASE_SETUP.md

**待后续完善**：
- 真实 LLM API 联调（需 API Key）
- zhparser 扩展验证（需 PG 环境）
- 性能测试脚本
- 安全测试脚本
- MCP 端到端测试

## [v0.6.3] - 2026-09-10

### BAAI Embedding/Reranker 模型名可配置 + 共享功能端到端验证

**背景**：`.env` 已写入 `BAAI_EMBEDDING_MODEL` / `BAAI_RERANKER_MODEL`，但代码未接线，模型名硬编码在 provider 内部；同时 `BAAI_BASE_URL` 示例值为 `https://api.deepinfra.com/v1`（带 `/v1`），而 provider 内部会再拼 `/v1/embeddings`，硅基流动（SiliconFlow）等不带 `/v1` 后缀的 base_url 接入时会出现双重 `/v1` 路径导致 404。

#### 新增 L1 配置项

- `BAAI_EMBEDDING_MODEL`：Embedding 模型名（默认 `BAAI/bge-large-zh-v1.5`）
- `BAAI_RERANKER_MODEL`：Reranker 模型名（默认 `BAAI/bge-reranker-v2-m3`）
- 两者均登记到 `managedKeys` / `defaultValues` / `YAMLConfig` / `toFlat`

#### Provider 改动

- `embedding_baai.go` / `reranker_baai.go`：构造器增加 `model` 参数；硬编码改为读成员变量
- 默认 base_url 由 `https://deepinfra.com/api` 改为 `https://api.deepinfra.com`（去掉尾部 `/api`，与 provider 路径拼接约定一致）

#### 配置示例修复

- `.env.example` / `config.yaml.example`：`BAAI_BASE_URL` 默认值去掉末尾 `/v1`（siliconflow 实测需 `/v1/embeddings` 直接拼在 base 之后）
- 补充 `BAAI_EMBEDDING_MODEL` / `BAAI_RERANKER_MODEL` 示例行

#### 共享功能端到端验证（agent-browser + curl）

- 登录后台 → 创建读写 API Key → 用 Key 录入文章 / 检索 / MCP 调用 → 全链路 200
- URL 采集流程：httpbin.org HTML 页面预览 → 入库 → AI 异步处理（LLM 摘要 + Embedding + BM25 索引）→ 检索命中，耗时 <200ms
- 鉴权隔离：无 Key 401 / 伪造 Key 401，scope 隔离正常
- 根因定位：此前 `EMBEDDING_PROVIDER=baai` 检索报 500（`embedding: 404`）系 `.env` 里 `BAAI_BASE_URL=https://api.siliconflow.cn/v1` 与 provider 内部拼接产生双重 `/v1`

**变更**：`internal/service/provider/embedding_baai.go`、`reranker_baai.go`、`factory.go`；`internal/config/config.go`；`cmd/aiknow-server/main.go`；`.env`、`.env.example`、`config.yaml.example`

**验证**：`go build ./...` / `go vet ./...` / `go test ./...` 全绿；agent-browser 真实浏览器端到端测试通过

## [v0.6.3] - 2026-09-10

### 修复：设置页分组显示 + URL 采集内容清洗

#### 1. 设置页按功能分组展示（修复前端未渲染问题）

**根因**：二进制嵌入的静态资源（`internal/handler/web/web/`）与源码目录（`web/`）长期不同步。
`web/index.html` 已更新至模块化架构（`app.js` 使用 `type="module"` 按顺序加载 Alpine 组件），
但嵌入的是旧的 827 行版本（无模块脚本、无 settings 分组模板），导致 `/ui/settings` 页
显示为登录提示而非分组配置表。

**修复**：将 `web/` 完整同步至 `internal/handler/web/web/`（含 index.html、js/、css/、login.html）。

**效果**：设置页现按 9 个分组正确渲染：LLM / Embedding / Reranker / 混合检索 /
文本分块 / 关键词检索 / MCP 日志 / 网页采集 / **提示词**（`prompt.analyze` 可在此直接编辑）。

#### 2. URL 采集内容清洗：移除 script/nav 等噪声，保留段落结构

**背景**：`pkg/htmlclean/htmlclean.go` 此前仅用正则 `<[^>]*>` 剥离所有 HTML 标签，
对含 `<script>`、`<nav>`、`<header>` 等噪声节点的页面，输出仍混入脚本代码片段；
且 `goquery.Text()` 不保留块元素间的换行，导致段落粘连（如 `HelloWorld`）。

**方案**：引入 `goquery`（HTML 解析）+ `golang.org/x/net/html`（DOM 遍历）：
- 语义定位正文区（`article` / `main` / `.content` 等选择器优先，退化为 `body`）
- 移除噪声节点（`script` / `style` / `nav` / `header` / `footer` / `aside` /
  广告类 class `.ad,.sidebar,.comment` 等）
- 递归遍历 DOM，块级元素（`p/div/h1-h6/ul/ol/li/blockquote/pre/hr/article/section` 等）
  前后插入换行，行内元素保持原样
- 解码常见 HTML 实体（`&lt;` / `&gt;` / `&amp;` / `&quot;` / `&#x...;` / `&#...;`）
- 压缩连续空白为单空格

**依赖新增**：`github.com/PuerkitoBio/goquery`、`golang.org/x/net`（后者已有传递依赖，显式声明）

**验证**：`httpbin.org/html`（无正文语义标签的简单 HTML）输出为纯文本段落，
不含 `<script>` / `var x=` 等噪声；`TestClean_BlockSpacing` / `TestClean_RemoveNoise` 新增用例通过。

**变更**：`pkg/htmlclean/htmlclean.go`（重写）、`pkg/htmlclean/htmlclean_test.go`（扩展）、
`internal/handler/web/web/`（同步 web/ 目录）、`.env.example`、`config.yaml.example`、
`internal/service/provider/embedding_baai.go`、`reranker_baai.go`、`factory.go`、
`internal/config/config.go`、`cmd/aiknow-server/main.go`（v0.6.2 BAAI 模型可配合并入本版本）

**验证**：`go test ./...` 全绿；agent-browser 端到端验证登录→设置页→采集→入库→检索→MCP 链路正常

## [v0.6.4] - 2026-09-10

### 修复：设置页分组显示 + URL 采集内容清洗 + 微信文章兼容

#### 1. 设置页按功能分组展示（修复前端未渲染问题）

**根因**：二进制嵌入的静态资源（`internal/handler/web/web/`）与源码目录（`web/`）长期不同步。
`web/index.html` 已更新至模块化架构（`app.js` 使用 `type="module"` 按顺序加载 Alpine 组件），
但嵌入的是旧的 827 行版本（无模块脚本、无 settings 分组模板），导致 `/ui/settings` 页
显示为登录提示而非分组配置表。

**修复**：将 `web/` 完整同步至 `internal/handler/web/web/`（含 index.html、js/、css/、login.html）。

**效果**：设置页现按 9 个分组正确渲染：LLM / Embedding / Reranker / 混合检索 /
文本分块 / 关键词检索 / MCP 日志 / 网页采集 / **提示词**（`prompt.analyze` 可在此直接编辑）。

#### 2. URL 采集内容清洗：移除 script/nav 等噪声，保留段落结构

**背景**：`pkg/htmlclean/htmlclean.go` 此前仅用正则 `<[^>]*>` 剥离所有 HTML 标签，
对含 `<script>`、`<nav>`、`<header>` 等噪声节点的页面，输出仍混入脚本代码片段；
且 `goquery.Text()` 不保留块元素间的换行，导致段落粘连（如 `HelloWorld`）。

**方案**：引入 `goquery`（HTML 解析）+ `golang.org/x/net/html`（DOM 遍历）：
- 语义定位正文区（`article`/`main`/`.content` 等选择器优先，退化为 `body`）
- 移除噪声节点（`script`/`style`/`nav`/`header`/`footer`/`aside`/广告类 class）
- 递归遍历 DOM，块级元素（`p/div/h1-h6/ul/ol/li/blockquote/pre/hr/article/section` 等）
  前后插入换行，行内元素保持原样
- 解码常见 HTML 实体（`&lt;`/`&gt;`/`&amp;`/`&quot;`/`&#x...;`/`&#...;`）
- 压缩连续空白为单空格

**依赖新增**：`github.com/PuerkitoBio/goquery`、`golang.org/x/net`（后者已有传递依赖，显式声明）

**验证**：`httpbin.org/html`（无正文语义标签的简单 HTML）输出为纯文本段落，
不含 `<script>`/`var x=` 等噪声；`TestClean_BlockSpacing`/`TestClean_RemoveNoise` 新增用例通过。

#### 3. 微信文章采集兼容

**背景**：微信公众号文章有反爬机制，默认 UA 会被 302 重定向拒绝；且 `js_content`
正文区在 1MB+ 处开始，默认 `MaxContentLength=100KB` 会截断。

**修复**：
- 默认 UA 改为 iPhone Safari（`Mozilla/5.0 (iPhone; CPU iPhone OS 17_0...)`），
  兼容微信、知乎、掘金等国内站点
- `MaxContentLength` 从 100KB 提升至 2MB，确保能读到 `js_content` 正文区
- htmlclean 新增 `#js_content` 选择器优先提取，适配微信公众号文章结构

**端到端验证**：
- 采集 `https://mp.weixin.qq.com/s/5VDN-T9K8Wr-DaQ15-I6CA`（Agent 自进化飞轮）
- 预览：22,289 字符，无脚本噪声
- 入库 → AI 处理（LLM 摘要 + Embedding 1024 维 + BM25 索引）→ 23 个分段
- REST 检索 "Agent 自进化"：命中该文章，耗时 849ms
- MCP `search_knowledge`：isError=false，返回相关片段
- 关键词检索 "Anthropic Agent"：命中该文章

**变更**：`pkg/htmlclean/htmlclean.go`（重写）、`pkg/htmlclean/htmlclean_test.go`（扩展）、
`internal/service/collector.go`（MaxContentLength + UA）、
`internal/handler/web/web/`（同步 web/ 目录）、
`.env.example`、`config.yaml.example`、
`internal/service/provider/embedding_baai.go`、`reranker_baai.go`、`factory.go`、
`internal/config/config.go`、`cmd/aiknow-server/main.go`（v0.6.2 BAAI 模型可配合并入本版本）

**验证**：`go test ./...` 全绿；agent-browser 端到端验证登录→设置页→采集→入库→检索→MCP 链路正常
