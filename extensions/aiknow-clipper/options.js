/** 选项页：只做两件事——存配置、用一次真实请求验证配置可用。 */

const DEFAULTS = { server: 'http://127.0.0.1:8080', apiKey: '' };
const $ = (id) => document.getElementById(id);

function setStatus(text, cls) {
  const el = $('status');
  el.textContent = text;
  el.className = cls || '';
}

async function load() {
  const cfg = await chrome.storage.local.get(DEFAULTS);
  $('server').value = cfg.server || DEFAULTS.server;
  $('apiKey').value = cfg.apiKey || '';
}

async function save() {
  const server = $('server').value.trim().replace(/\/+$/, '');
  const apiKey = $('apiKey').value.trim();
  await chrome.storage.local.set({ server, apiKey });
  setStatus('已保存', 'ok');
}

async function test() {
  const server = $('server').value.trim().replace(/\/+$/, '');
  const apiKey = $('apiKey').value.trim();
  if (!apiKey) return setStatus('请先填 API Key', 'err');

  setStatus('测试中…');
  try {
    // 用一次只读请求验证「地址可达 + Key 有效」
    const res = await fetch(`${server}/api/v1/articles?page=1&page_size=1`, {
      headers: { Authorization: `Bearer ${apiKey}` },
    });
    if (res.ok) return setStatus('连接正常，Key 有效', 'ok');
    if (res.status === 401) return setStatus('Key 无效或已被禁用', 'err');
    return setStatus(`服务返回 HTTP ${res.status}`, 'err');
  } catch (e) {
    setStatus(`连不上：${e.message}`, 'err');
  }
}

$('save').addEventListener('click', save);
$('test').addEventListener('click', test);
load();
