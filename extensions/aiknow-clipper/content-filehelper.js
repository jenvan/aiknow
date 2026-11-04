/**
 * 文件传输助手网页版 → AiKnow 自动入库。
 *
 * 背景：微信「转发」列表里有「文件传输助手」，但**没有 ClawBot**（它是插件会话），
 * 所以手机上用原生转发给文件传输助手，网页版这边由脚本把链接抽出来入库。
 *
 * 实测要点（2026-11，均在真实页面上验证过）：
 *   1. 登录后域名会从 filehelper.weixin.qq.com 变成 **szfilehelper.weixin.qq.com**，
 *      manifest 里两个都要匹配，否则脚本根本不加载。
 *   2. 刷新页面后消息列表会清空（只剩引导文案），但**下一条新消息到达时，
 *      历史会整批回填**——所以「打开页面后补录」是正常路径，不是 bug。
 *   3. 消息节点是整批重建的（不是逐个 append），所以必须「全量扫描 + 已见集合去重」，
 *      只监听新增节点会漏。
 *
 * 两条安全约定（务必保持）：
 *   - **只发链接，不发 DOM**：头像/封面 img 的 src 带 skey=@crypt_…，是网页会话
 *     凭据，发给服务端或写进日志都等于泄露登录态。
 *   - **只自动收 mp.weixin.qq.com**：文件传输助手还会传文件、传别的链接，
 *     全收会把知识库塞满噪声；其它网页请用右键菜单手动剪藏。
 */

const SEEN_KEY = 'aiknow-filehelper-seen'; // localStorage：跨标签页共享，避免重复提交
const MAX_SEEN = 500;

function loadSeen() {
  try {
    return new Set(JSON.parse(localStorage.getItem(SEEN_KEY) || '[]'));
  } catch {
    return new Set();
  }
}

const seen = loadSeen();

function remember(url) {
  seen.add(url);
  try {
    localStorage.setItem(SEEN_KEY, JSON.stringify(Array.from(seen).slice(-MAX_SEEN)));
  } catch {
    /* 存不下就只保留内存态 */
  }
}

function isWeixinArticle(url) {
  try {
    return new URL(url).host === 'mp.weixin.qq.com';
  } catch {
    return false;
  }
}

/** 角标：贴在卡片右下角，不改变原有布局 */
function badge(card, text, color) {
  const existing = card.querySelector('.aiknow-badge');
  if (existing) existing.remove();
  const span = document.createElement('span');
  span.className = 'aiknow-badge';
  span.textContent = text;
  span.style.cssText =
    'position:absolute;top:22px;right:6px;padding:1px 6px;border-radius:9px;' +
    `font-size:11px;line-height:16px;color:#fff;background:${color};z-index:10;`;
  card.style.position = 'relative';
  card.appendChild(span);
}

/**
 * 串行队列。
 *
 * 服务端采集是同步的（渲染开启时单篇可达 30s），并行提交会让 10 篇文章同时排队
 * 渲染、把 browserless 打爆，所以一次只发一个。
 */
let chain = Promise.resolve();
function enqueue(task) {
  chain = chain.then(task).catch((e) => console.warn('[AiKnow] 任务失败：', e));
  return chain;
}

function submit(card, url) {
  if (seen.has(url)) return;
  remember(url);
  badge(card, '排队中…', '#6b7280');

  enqueue(async () => {
    badge(card, '入库中…', '#6b7280');
    const result = await chrome.runtime.sendMessage({ type: 'aiknow-submit', url });
    if (!result) return badge(card, '扩展未就绪', '#dc2626');
    if (result.ok) return badge(card, '✓ 已收录', '#16a34a');
    if (result.dup) return badge(card, '= 已存在', '#d97706');
    badge(card, '! 失败', '#dc2626');
    card.title = result.message || '';
    console.warn('[AiKnow] 入库失败：', result.message);
  });
}

/** 一张公众号文章卡片 */
function scanCard(anchor) {
  const href = anchor.getAttribute('href');
  if (!href || !isWeixinArticle(href)) return;
  const item = anchor.closest('.msg-appmsg') || anchor;
  submit(item, href);
}

/** 纯文本消息里粘的公众号链接（只扫自己发的） */
function scanText(node) {
  const matches = (node.textContent || '').match(/https?:\/\/mp\.weixin\.qq\.com\/[^\s"'<>]+/g);
  if (!matches) return;
  for (const raw of matches) {
    const url = raw.replace(/[.,;!?，。；！？、]+$/, '');
    if (seen.has(url)) continue;
    submit(node, url);
  }
}

function scan() {
  document.querySelectorAll('a.msg-appmsg__content').forEach(scanCard);
  document.querySelectorAll('.msg-item.mine .msg-item__content').forEach((el) => {
    if (el.querySelector('a.msg-appmsg__content')) return; // 卡片已单独处理
    scanText(el);
  });
}

let timer = null;
function schedule() {
  if (timer) return;
  timer = setTimeout(() => {
    timer = null;
    scan();
  }, 400);
}

new MutationObserver(schedule).observe(document.body, { childList: true, subtree: true });

// 页面打开时先扫一遍：历史消息通常在「下一条新消息」触发同步后整批回填，
// 所以这一步是补录的主路径，不是异常分支。
schedule();
console.info('[AiKnow] 文件传输助手监听已启动');
