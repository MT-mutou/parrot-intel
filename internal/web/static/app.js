/* 鹦鹉情报站 · 交互脚本（~3KB，零依赖） */
'use strict';

function $(s, p) { return (p || document).querySelector(s); }
function $$(s, p) { return Array.from((p || document).querySelectorAll(s)); }

var toastTimer = null;
function toast(msg, isError) {
  var t = $('#toast');
  t.textContent = msg;
  t.className = 'toast show' + (isError ? ' error' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(function () { t.className = 'toast'; }, 3500);
}

function api(method, url, body) {
  return fetch(url, {
    method: method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined
  }).then(function (r) {
    return r.json().catch(function () { return {}; }).then(function (d) {
      if (!r.ok) throw new Error(d.error || ('HTTP ' + r.status));
      return d;
    });
  });
}

/* ---- 收集 ---- */
function doCollect(btn) {
  btn.disabled = true;
  btn.textContent = '⟳ 启动中…';
  api('POST', '/api/collect').then(function (d) {
    toast(d.started ? '收集已启动，后台运行中' : (d.msg || '收集已在进行中'));
    if (d.started) pollStatus();
    else { btn.disabled = false; btn.textContent = '立即收集'; }
  }).catch(function (e) {
    toast(e.message, true);
    btn.disabled = false; btn.textContent = '立即收集';
  });
}

var polling = false;
function pollStatus() {
  if (polling) return;
  polling = true;
  var badge = $('#col-badge'), btn = $('#btn-collect');
  if (btn) { btn.disabled = true; btn.textContent = '⟳ 收集中'; }
  (function tick() {
    api('GET', '/api/collect/status').then(function (d) {
      if (badge) { badge.className = 'col-badge' + (d.running ? ' running' : ''); badge.textContent = d.running ? '⟳ 收集中' : '● 在线'; badge.title = d.status || ''; }
      if (d.running) { setTimeout(tick, 2000); return; }
      polling = false;
      if (btn) { btn.disabled = false; btn.textContent = '立即收集'; }
      if (d.status && d.status.indexOf('完成') === 0) { toast(d.status); setTimeout(function () { location.reload(); }, 1200); }
    }).catch(function () { polling = false; if (btn) { btn.disabled = false; btn.textContent = '立即收集'; } });
  })();
}
if ($('#col-badge') && $('#col-badge').classList.contains('running')) pollStatus();

/* ---- 收藏/已读 ---- */
function toggleStar(el, id) {
  el.classList.toggle('on');
  api('POST', '/api/star/' + id).then(function (d) {
    el.classList.toggle('on', d.starred);
    toast(d.starred ? '已收藏 ★' : '已取消收藏');
  }).catch(function (e) { el.classList.toggle('on'); toast(e.message, true); });
}

/* ---- 单篇 AI 分析 ---- */
function analyzeArticle(btn, id) {
  btn.disabled = true; btn.textContent = '⟳ AI 分析中…';
  api('POST', '/api/analyze/' + id).then(function () {
    toast('分析完成，刷新查看');
    setTimeout(function () { location.reload(); }, 800);
  }).catch(function (e) { btn.disabled = false; btn.textContent = '⚡ AI 深度分析'; toast(e.message, true); });
}

/* ---- 简报 ---- */
function genBriefing(btn) {
  btn.disabled = true; btn.textContent = '⟳ AI 生成中（约 1 分钟）…';
  api('POST', '/api/briefing/generate').then(function (d) {
    toast('简报已生成');
    $('#briefing').innerHTML = '<div class="md">' + mdLite(d.content) + '</div>';
    btn.disabled = false; btn.textContent = '⚡ 重新生成';
  }).catch(function (e) { btn.disabled = false; btn.textContent = '⚡ AI 生成今日简报'; toast(e.message, true); });
}

function pushBriefing(btn) {
  btn.disabled = true; btn.textContent = '⟳ 推送中…';
  api('POST', '/api/briefing/push').then(function () {
    toast('已推送到钉钉群 ✓');
    btn.disabled = false; btn.textContent = '🔔 推送到钉钉';
  }).catch(function (e) { btn.disabled = false; btn.textContent = '🔔 推送到钉钉'; toast(e.message, true); });
}

/* 极简 markdown 渲染（简报前端局部刷新用） */
function esc(s) { return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }
function mdLite(md) {
  return md.split('\n').map(function (ln) {
    ln = esc(ln);
    ln = ln.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    if (/^### /.test(ln)) return '<h4>' + ln.slice(4) + '</h4>';
    if (/^## /.test(ln)) return '<h3>' + ln.slice(3) + '</h3>';
    if (/^# /.test(ln)) return '<h2>' + ln.slice(2) + '</h2>';
    if (/^[-*] /.test(ln)) return '<li>' + ln.slice(2) + '</li>';
    if (/^&gt; /.test(ln)) return '<blockquote>' + ln.slice(5) + '</blockquote>';
    if (!ln.trim()) return '';
    return '<p>' + ln + '</p>';
  }).join('');
}

/* ---- 来源管理 ---- */
function addSource(btn) {
  var name = $('#s-name').value.trim(), url = $('#s-url').value.trim(), cat = $('#s-cat').value.trim();
  if (!name || !url) { toast('名称和 RSS 地址必填', true); return; }
  btn.disabled = true;
  api('POST', '/api/sources', { action: 'add', name: name, url: url, category: cat }).then(function () {
    toast('已添加 ' + name);
    setTimeout(function () { location.reload(); }, 600);
  }).catch(function (e) { btn.disabled = false; toast(e.message, true); });
}
function toggleSource(id) {
  api('POST', '/api/sources', { action: 'toggle', id: id }).then(function () { location.reload(); })
    .catch(function (e) { toast(e.message, true); });
}
function deleteSource(id, name) {
  if (!confirm('确认删除来源「' + name + '」？已收集的文章会保留。')) return;
  api('POST', '/api/sources', { action: 'delete', id: id }).then(function () { location.reload(); })
    .catch(function (e) { toast(e.message, true); });
}
