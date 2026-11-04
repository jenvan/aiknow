/**
 * AiKnow 剪藏 —— 后台脚本（MV3 service worker）
 *
 * 职责只有两件：
 *   1. 右键菜单 / 点击图标 → 把当前页 URL 交给 AiKnow；
 *   2. 提供统一的提交函数给 content script 复用（通过消息传递）。
 *
 * 配置（服务地址 + API Key）存在 chrome.storage.local，需要用户在选项页填一次。
 */

const DEFAULT_CONFIG = { server: 'http://127.0.0.1:8080', apiKey: '' };

async function getConfig() {
  const saved = await chrome.storage.local.get(DEFAULT_CONFIG);
  return { ...DEFAULT_CONFIG, ...saved };
}

/**
 * 提交一个 URL 到 AiKnow。
 * 返回 { ok, message }：
 *   ok=true  首次入库
 *   ok=false 失败（dup=true 表示已存在，不算错误）
 */
async function submitURL(url) {
  const { server, apiKey } = await getConfig();
  if (!apiKey) {
    return { ok: false, message: '未配置 API Key，请在扩展选项页填写' };
  }

  const base = (server || '').replace(/\/+$/, '');
  let res;
  try {
    res = await fetch(`${base}/api/v1/articles/from-url`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({ url }),
    });
  } catch (e) {
    return { ok: false, message: `连不上 AiKnow（${base}）：${e.message}` };
  }

  // 409 = 已存在：这是正常业务结果，不该报错
  if (res.status === 409) {
    return { ok: false, dup: true, message: '已收录过，未重复入库' };
  }
  if (res.status === 201) {
    const article = await res.json().catch(() => ({}));
    return { ok: true, message: `已收录：${article.title || url}` };
  }

  let detail = '';
  try {
    detail = (await res.json()).error || '';
  } catch {
    /* 忽略非 JSON 响应 */
  }
  return { ok: false, message: `入库失败（HTTP ${res.status}）${detail}` };
}

function flash(ok, dup) {
  // 用图标角标做轻量反馈，避免额外权限
  const text = ok ? '✓' : dup ? '=' : '!';
  chrome.action.setBadgeText({ text });
  chrome.action.setBadgeBackgroundColor({ color: ok ? '#16a34a' : dup ? '#d97706' : '#dc2626' });
  setTimeout(() => chrome.action.setBadgeText({ text: '' }), 3000);
}

async function clipURL(url) {
  const result = await submitURL(url);
  flash(result.ok, result.dup);
  return result;
}

// 右键菜单：任意页面可用
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: 'aiknow-save',
    title: '存入 AiKnow 知识库',
    contexts: ['page', 'link', 'selection'],
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.linkUrl || tab?.url;
  if (url) await clipURL(url);
});

// 点击工具栏图标：剪藏当前页
chrome.action.onClicked.addListener(async (tab) => {
  if (tab?.url) await clipURL(tab.url);
});

// content script 的提交走这里：统一配置与错误处理
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type !== 'aiknow-submit') return false;
  clipURL(message.url).then(sendResponse);
  return true; // 保持异步响应通道
});
