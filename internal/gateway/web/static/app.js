(function () {
  var token = '';
  var currentSessionKey = getSessionKey();

  var statusEl = document.getElementById('status');
  var tokenEl = document.getElementById('token');
  var connectBtn = document.getElementById('connectBtn');
  var chatHistory = document.getElementById('chatHistory');
  var chatInput = document.getElementById('chatInput');
  var sendBtn = document.getElementById('sendBtn');
  var refreshSessions = document.getElementById('refreshSessions');
  var sessionsList = document.getElementById('sessionsList');
  var refreshConfig = document.getElementById('refreshConfig');
  var configInfo = document.getElementById('configInfo');
  var healthInfo = document.getElementById('healthInfo');

  function setStatus(text, className) {
    statusEl.textContent = text;
    statusEl.className = 'status ' + (className || '');
  }

  function getDeviceId() {
    try {
      var id = localStorage.getItem('aido_device_id');
      if (id) return id;
      var oldKey = sessionStorage.getItem('aido_session_key');
      if (oldKey && oldKey.indexOf('webchat:default:') === 0) {
        id = oldKey.slice('webchat:default:'.length);
        localStorage.setItem('aido_device_id', id);
        return id;
      }
      id = 'web-' + Math.random().toString(36).slice(2) + '-' + Date.now().toString(36);
      localStorage.setItem('aido_device_id', id);
      return id;
    } catch (e) {
      return 'web-default';
    }
  }

  function getSessionKey() {
    return 'webchat:default:' + getDeviceId();
  }

  function apiCall(path, options) {
    options = options || {};
    var h = { 'Content-Type': 'application/json' };
    if (token) h['Authorization'] = 'Bearer ' + token;
    return fetch('/api' + path, {
      method: options.method || 'GET',
      headers: h,
      body: options.body ? JSON.stringify(options.body) : undefined
    }).then(function (r) {
      if (!r.ok) return null;
      return r.json();
    }).catch(function () { return null; });
  }

  function connect() {
    token = tokenEl.value.trim();
    if (!token) {
      setStatus('请填写 Token', 'error');
      return;
    }
    apiCall('/health').then(function (res) {
      if (res && res.status === 'ok') {
        currentSessionKey = getSessionKey();
        setStatus('已连接', 'connected');
        connectBtn.textContent = '已连接';
        connectBtn.disabled = true;
        try {
          sessionStorage.setItem('aido_token', token);
          localStorage.setItem('aido_token', token);
        } catch (e) {}
        loadChatHistory();
        loadHealth();
        loadSessions();
        loadConfig();
      } else {
        setStatus('认证失败', 'error');
      }
    }).catch(function () {
      setStatus('连接失败', 'error');
    });
  }

  connectBtn.addEventListener('click', connect);

  (function tryAutoConnectFromHash() {
    var hash = location.hash.slice(1);
    if (!hash) return;
    var params = new URLSearchParams(hash);
    var t = params.get('token');
    if (t) {
      tokenEl.value = t;
      connect();
    }
  })();

  function appendMessage(role, text) {
    if (!text) return;
    var div = document.createElement('div');
    div.className = 'msg ' + role;
    var body = role === 'assistant' ? markdownToHtml(text) : escapeHtml(text);
    div.innerHTML = '<div class="role">' + (role === 'user' ? '我' : 'Aido') + '</div><div class="msg-body">' + body + '</div>';
    chatHistory.appendChild(div);
    chatHistory.scrollTop = chatHistory.scrollHeight;
  }

  function appendAssistantMessage(text, toolSteps) {
    var div = document.createElement('div');
    div.className = 'msg assistant';
    var parts = [];
    if (toolSteps && toolSteps.length > 0) {
      var stepsHtml = '<details class="tool-steps"><summary>工具调用 (' + toolSteps.length + ')</summary>';
      toolSteps.forEach(function (step, i) {
        var params = (step.toolParams || '').trim();
        var result = (step.toolResult || '').trim();
        stepsHtml += '<details class="tool-step"><summary>' + escapeHtml(step.toolName || 'tool') + '</summary>';
        if (params) stepsHtml += '<div class="tool-meta"><span class="tool-meta-label">参数</span><pre class="tool-params">' + escapeHtml(params) + '</pre></div>';
        stepsHtml += '<div class="tool-meta"><span class="tool-meta-label">结果</span><pre class="tool-result">' + escapeHtml(result) + '</pre></div></details>';
      });
      stepsHtml += '</details>';
      parts.push(stepsHtml);
    }
    if (text) parts.push(markdownToHtml(text));
    div.innerHTML = '<div class="role">Aido</div><div class="msg-body">' + parts.join('') + '</div>';
    chatHistory.appendChild(div);
    chatHistory.scrollTop = chatHistory.scrollHeight;
  }

  function loadChatHistory() {
    if (!currentSessionKey) return;
    apiCall('/chat/history?sessionKey=' + encodeURIComponent(currentSessionKey)).then(function (res) {
      if (!res || !res.messages) return;
      chatHistory.innerHTML = '';
      res.messages.forEach(function (msg) {
        var role = msg.role;
        var content = typeof msg.content === 'string' ? msg.content : (msg.content && msg.content[0] && msg.content[0].text) ? msg.content[0].text : '';
        if (role === 'user' || role === 'assistant') appendMessage(role, content);
      });
      chatHistory.scrollTop = chatHistory.scrollHeight;
    });
  }

  function escapeHtml(s) {
    var div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }

  function markdownToHtml(md) {
    if (!md) return '';
    var out = [];
    var lines = md.split('\n');
    var i = 0;
    var inBlock = false;
    var blockBuf = [];
    function flushBlock() {
      if (blockBuf.length) {
        out.push('<pre><code>' + escapeHtml(blockBuf.join('\n')) + '</code></pre>');
        blockBuf = [];
      }
      inBlock = false;
    }
    while (i < lines.length) {
      var line = lines[i];
      var trimmed = line.trim();
      if (line.startsWith('```')) {
        flushBlock();
        if (!inBlock) { inBlock = true; i++; continue; }
        i++;
        continue;
      }
      if (inBlock) {
        blockBuf.push(line);
        i++;
        continue;
      }
      if (trimmed === '') {
        flushBlock();
        out.push('<br>');
        i++;
        continue;
      }
      var m = trimmed.match(/^(#{1,6})\s+(.+)$/);
      if (m) {
        flushBlock();
        var l = m[1].length;
        out.push('<h' + l + '>' + inlineMarkdown(m[2]) + '</h' + l + '>');
        i++;
        continue;
      }
      if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
        flushBlock();
        out.push('<ul><li>' + inlineMarkdown(trimmed.slice(2)) + '</li></ul>');
        i++;
        continue;
      }
      if (trimmed.startsWith('> ')) {
        flushBlock();
        out.push('<blockquote>' + inlineMarkdown(trimmed.slice(2)) + '</blockquote>');
        i++;
        continue;
      }
      out.push('<p>' + inlineMarkdown(trimmed) + '</p>');
      i++;
    }
    flushBlock();
    return out.join('');
  }
  function inlineMarkdown(t) {
    var re = /\*\*(.+?)\*\*|\*(.+?)\*|_(.+?)_|`([^`]+)`|\[([^\]]+)\]\(([^)]+)\)/g;
    var out = [];
    var last = 0;
    var match;
    while ((match = re.exec(t)) !== null) {
      out.push(escapeHtml(t.slice(last, match.index)));
      if (match[1] !== undefined) out.push('<strong>' + escapeHtml(match[1]) + '</strong>');
      else if (match[2] !== undefined) out.push('<em>' + escapeHtml(match[2]) + '</em>');
      else if (match[3] !== undefined) out.push('<em>' + escapeHtml(match[3]) + '</em>');
      else if (match[4] !== undefined) out.push('<code>' + escapeHtml(match[4]) + '</code>');
      else if (match[5] !== undefined) out.push('<a href="' + escapeHtml(match[6]) + '" target="_blank" rel="noopener">' + escapeHtml(match[5]) + '</a>');
      last = re.lastIndex;
    }
    out.push(escapeHtml(t.slice(last)));
    return out.join('');
  }

  sendBtn.addEventListener('click', function () {
    var text = chatInput.value.trim();
    if (!text || !token) return;
    appendMessage('user', text);
    chatInput.value = '';
    apiCall('/chat/send', { method: 'POST', body: { text: text, sessionKey: currentSessionKey } }).then(function (res) {
      if (res) appendAssistantMessage(res.text || '', res.toolSteps);
    });
  });

  chatInput.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendBtn.click();
    }
  });

  function loadSessions() {
    if (!token) {
      sessionsList.innerHTML = '<div class="session-item">请先连接</div>';
      return;
    }
    apiCall('/sessions').then(function (res) {
      if (!res || !res.sessions) {
        sessionsList.innerHTML = '<div class="session-item">暂无会话或未连接</div>';
        return;
      }
      sessionsList.innerHTML = res.sessions.map(function (s) {
        return '<div class="session-item"><span class="session-key">' + escapeHtml(s.sessionKey) + '</span><div class="session-meta">' +
          'Agent: ' + escapeHtml(s.agentId || '') + ' | 更新: ' + (s.updatedAt || '') + '</div></div>';
      }).join('') || '<div class="session-item">暂无会话</div>';
    });
  }

  refreshSessions.addEventListener('click', loadSessions);

  function loadHealth() {
    if (!token) {
      healthInfo.textContent = '请先连接';
      return;
    }
    apiCall('/health').then(function (res) {
      healthInfo.textContent = res ? JSON.stringify(res, null, 2) : '获取失败';
    });
  }

  function loadConfig() {
    if (!token) {
      configInfo.textContent = '请先连接';
      return;
    }
    apiCall('/config').then(function (res) {
      configInfo.textContent = res ? JSON.stringify(res, null, 2) : '获取失败';
    });
  }

  refreshConfig.addEventListener('click', loadConfig);

  document.querySelectorAll('.tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
      document.querySelectorAll('.tab').forEach(function (t) { t.classList.remove('active'); });
      document.querySelectorAll('.panel').forEach(function (p) { p.classList.remove('active'); });
      tab.classList.add('active');
      document.getElementById('panel-' + tab.dataset.tab).classList.add('active');
      if (tab.dataset.tab === 'health') loadHealth();
      if (tab.dataset.tab === 'config') loadConfig();
      if (tab.dataset.tab === 'sessions') loadSessions();
      if (tab.dataset.tab === 'chat' && token) loadChatHistory();
    });
  });

  (function tryRestoreToken() {
    var hash = location.hash.slice(1);
    if (hash && new URLSearchParams(hash).get('token')) return;
    try {
      var saved = sessionStorage.getItem('aido_token') || localStorage.getItem('aido_token');
      if (saved) {
        tokenEl.value = saved;
        token = saved;
        loadChatHistory();
        connect();
      }
    } catch (e) {}
  })();
})();
