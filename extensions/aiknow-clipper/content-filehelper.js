/**
 * 文件传输助手网页版 → AiKnow 自动入库。
 *
 * 工作原理：微信「转发」列表里有「文件传输助手」（ClawBot 没有），所以手机上的
 * 操作是原生转发；本脚本盯住网页版新增的消息，把公众号文章卡片的链接抽出来入库。
 *
 * 两条安全约定（务必保持）：
 *   1. **只发链接，不发 DOM**。页面里的头像/封面 img 的 src 带 skey=@crypt_…，
 *      那是网页会话凭据，绝不能发给服务端、也不能写进日志。
 *   2. **只自动收公众号（mp.weixin.qq.com）**。文件传输助手也可能传文件、传别的
 *      链接，自动全收会把知识库塞满噪声；其它网页请用右键菜单手动剪藏。
 */

const SEEN_KEY = 'aiknow-filehelper-seen';
const MAX_SEEN = 500;

const seen = new Set(JSON.parse(sessionStorage.getItem(SEEN_KEY) || '[]'));

function remember(url) {
  seen.add(url);
  const arr = Array.from(seen).slice(-MAX_SEEN);
  sessionStorage.setItem(SEEN_KEY, JSON.stringify(arr));
}

function isWeixinArticle(url) {
  try {
    return new URL(url).host === 'mp.weixin.qq.com';
  } catch {
    return false;
  }
}

/** 角标：贴在卡片左上角，不改变原有布局 */
function badge(card, text, color) {
  const existing = card.querySelector('.aiknow-badge');
  if (existing) existing.remove();
  const span = document.createElement('span');
  span.className = 'aiknow-badge';
  span.textContent = text;
  span.style.cssText =
    `position:absolute;top:4px;right:4px;padding:1px 6px;border-radius:9px;` +
    `font-size:11px;line-height:16px;color:#fff;background:${color};z-index:10;`;
  card.style.position = 'relative';
  card.appendChild(span);
}

async function submit(card, url, title) {
  if (seen.has(url)) return;
  remember(url);
  badge(card, '入库中…', '#6b7280');

  const result = await chrome.runtime.sendMessage({ type: 'aiknow-submit', url });
  if (!result) {
    badge(card, '扩展未就绪', '#dc2626');
    return;
  }
  if (result.ok) {
    badge(card, '✓ 已收录', '#16a34a');
  } else if (result.dup) {
    badge(card, '= 已存在', '#d97706');
  } else {
    badge(card, '! 失败', '#dc2626');
    card.title = result.message || '';
    console.warn('[AiKnow] 入库失败：', result.message);
  }
}

/** 扫描一张公众号文章卡片 */
function scanCard(anchor) {
  const href = anchor.getAttribute('href');
  if (!href || !isWeixinArticle(href)) return;

  const item = anchor.closest('.msg-appmsg') || anchor;
  const titleEl = anchor.querySelector('.msg-appmsg__title');
  const title = titleEl ? titleEl.textContent.trim() : '';
  submit(item, href, title);
}

/** 扫描纯文本消息里粘的公众号链接 */
function scanText(node) {
  const text = node.textContent || '';
  const matches = text.match(/https?:\/\/mp\.weixin\.qq\.com\/[^\s"'<>]+/g);
  if (!matches) return;
  for (const raw of matches) {
    const url = raw.replace(/[.,;!?，。；！？、]+$/, '');
    if (seen.has(url)) continue;
    submit(node, url);
  }
}

function scan() {
  document.querySelectorAll('a.msg-appmsg__content').forEach(scanCard);
  // 只扫「我的」消息（自己发给文件传输助手的），别人的不受影响
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

// 页面打开时先扫一遍：把「扩展装之前/页面刷新前」就存在的卡片也补进来
schedule();
console.info('[AiKnow] 文件传输助手监听已启动');
