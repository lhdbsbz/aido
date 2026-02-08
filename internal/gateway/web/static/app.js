(function () {
  var token = '';
  var currentChannel = 'webchat';
  var currentChannelChatId = getDeviceId();
  var ws = null;
  var pending = {};
  var nextReqId = 1;
  var agentEventCallback = null;
  var passiveStreamDiv = null;
  var historyPollTimer = null;
  var historyPollPlaceholder = null;
  var closedByUser = false;
  var reconnectAttempts = 0;
  var maxReconnectAttempts = 5;

  var statusEl = document.getElementById('status');
  var tokenEl = document.getElementById('token');
  var connectBtn = document.getElementById('connectBtn');
  var chatHistory = document.getElementById('chatHistory');
  var chatInput = document.getElementById('chatInput');
  var sendBtn = document.getElementById('sendBtn');
  var executionLogSwitch = document.getElementById('executionLogSwitch');
  var refreshSessions = document.getElementById('refreshSessions');
  var sessionsList = document.getElementById('sessionsList');
  var refreshConfig = document.getElementById('refreshConfig');
  var saveConfig = document.getElementById('saveConfig');
  var configMessage = document.getElementById('configMessage');
  var configPathDisplay = document.getElementById('configPathDisplay');
  var configGatewayPort = document.getElementById('configGatewayPort');
  var configGatewayToken = document.getElementById('configGatewayToken');
  var configGatewayCurrentAgent = document.getElementById('configGatewayCurrentAgent');
  var configGatewayToolsProfile = document.getElementById('configGatewayToolsProfile');
  var configGatewayLocale = document.getElementById('configGatewayLocale');
  var configAgents = document.getElementById('configAgents');
  var configAgentAdd = document.getElementById('configAgentAdd');
  var configProviders = document.getElementById('configProviders');
  var configProviderAdd = document.getElementById('configProviderAdd');
  var configMCP = document.getElementById('configMCP');
  var configMCPAdd = document.getElementById('configMCPAdd');
  var configBridges = document.getElementById('configBridges');
  var configBridgeAdd = document.getElementById('configBridgeAdd');
  var healthInfo = document.getElementById('healthInfo');

  var currentConfig = null;

  function getWsUrl() {
    var scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return scheme + '//' + location.host + '/ws';
  }

  function wsRequest(method, params, timeoutMs) {
    timeoutMs = timeoutMs || 30000;
    var id = String(nextReqId++);
    return new Promise(function (resolve, reject) {
      var t = setTimeout(function () {
        if (pending[id]) {
          delete pending[id];
          reject(new Error('timeout'));
        }
      }, timeoutMs);
      pending[id] = function (ok, data) {
        clearTimeout(t);
        if (ok) resolve(data); else reject(data || new Error('request failed'));
      };
      if (!ws || ws.readyState !== 1) {
        delete pending[id];
        reject(new Error('WebSocket not connected'));
        return;
      }
      ws.send(JSON.stringify({ type: 'req', id: id, method: method, params: params || {} }));
    });
  }

  function onWsMessage(ev) {
    var msg;
    try {
      msg = JSON.parse(ev.data);
    } catch (e) { return; }
    if (msg.type === 'res') {
      var cb = pending[msg.id];
      if (cb) {
        delete pending[msg.id];
        var ok = msg.ok === true;
        var data = null;
        if (ok && msg.payload) {
          try {
            data = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
          } catch (e) {}
        } else if (!ok && msg.error) {
          data = { code: msg.error.code, message: msg.error.message };
        }
        cb(ok, data);
      }
      return;
    }
    if (msg.type === 'event' && msg.event === 'agent' && msg.payload) {
      var payload;
      try {
        payload = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
      } catch (e) { return; }
      if (agentEventCallback) {
        agentEventCallback(payload);
      } else {
        handlePassiveAgentEvent(payload);
      }
      return;
    }
    if (msg.type === 'event' && msg.event === 'user_message' && msg.payload) {
      var payload;
      try {
        payload = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
      } catch (e) { return; }
      if (payload.text != null && payload.channel === currentChannel && payload.channelChatId === currentChannelChatId) {
        appendMessage('user', payload.text);
      }
    }
  }

  function eventMatchesCurrentConversation(ev) {
    return ev && ev.channel === currentChannel && ev.channelChatId === currentChannelChatId;
  }

  function handlePassiveAgentEvent(ev) {
    if (!eventMatchesCurrentConversation(ev)) return;
    if (ev.type === 'stream_start' && !passiveStreamDiv) {
      passiveStreamDiv = document.createElement('div');
      passiveStreamDiv.className = 'msg assistant streaming';
      passiveStreamDiv.innerHTML = '<div class="msg-head">' + AVATAR_ASSISTANT + '<span class="role">Aido</span></div><details class="execution-log"><summary>执行过程</summary><div class="execution-log-inner"></div></details><div class="msg-body"></div>';
      chatHistory.appendChild(passiveStreamDiv);
      applyExecutionLogState();
      chatHistory.scrollTop = chatHistory.scrollHeight;
    }
    if (!passiveStreamDiv) return;
    var logEl = passiveStreamDiv.querySelector('.execution-log-inner');
    var body = passiveStreamDiv.querySelector('.msg-body');
    if (ev.type === 'stream_start' && logEl) {
      appendExecutionLog(logEl, 'status', escapeHtml(EXEC.start));
      chatHistory.scrollTop = chatHistory.scrollHeight;
    } else if (ev.type === 'tool_start' && logEl) {
      var params = (ev.toolParams || '').trim();
      var short = params.length > 80 ? params.slice(0, 80) + '…' : params;
      appendExecutionLog(logEl, 'tool-start', EXEC.call + escapeHtml(ev.toolName || 'tool') + (short ? ': ' + escapeHtml(short) : ''));
      chatHistory.scrollTop = chatHistory.scrollHeight;
    } else if (ev.type === 'tool_end' && logEl) {
      var res = (ev.toolResult || '').trim();
      var resShort = res.length > 120 ? res.slice(0, 120) + '…' : res;
      appendExecutionLog(logEl, 'tool-end', EXEC.return + (resShort ? escapeHtml(resShort) : '—'));
      chatHistory.scrollTop = chatHistory.scrollHeight;
    } else if (ev.type === 'text_delta' && ev.text && body) {
      body.innerHTML += escapeHtml(ev.text);
      chatHistory.scrollTop = chatHistory.scrollHeight;
    } else if (ev.type === 'done' && logEl) {
      appendExecutionLog(logEl, 'status', escapeHtml(EXEC.done));
      removeExecutionLogIfEmpty(logEl);
      passiveStreamDiv.classList.remove('streaming');
      chatHistory.scrollTop = chatHistory.scrollHeight;
      passiveStreamDiv = null;
      loadChatHistory();
    } else if (ev.type === 'error' && ev.error && logEl) {
      appendExecutionLog(logEl, 'error', EXEC.error + escapeHtml(ev.error));
      chatHistory.scrollTop = chatHistory.scrollHeight;
    }
  }

  function closeWs() {
    closedByUser = true;
    agentEventCallback = null;
    passiveStreamDiv = null;
    if (ws) {
      ws.onclose = null;
      ws.onmessage = null;
      ws.onerror = null;
      ws.close();
      ws = null;
    }
    Object.keys(pending).forEach(function (id) {
      var cb = pending[id];
      if (cb) cb(false, { message: 'connection closed' });
    });
    pending = {};
  }

  var OPENAI_MODELS = [
    'gpt-4o', 'gpt-4o-mini', 'gpt-4o-n', 'gpt-4-turbo', 'gpt-4', 'gpt-3.5-turbo',
    'o1', 'o1-mini'
  ];
  var ANTHROPIC_MODELS = [
    'claude-sonnet-4-20250514', 'claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022',
    'claude-3-opus-20240229', 'claude-3-sonnet-20240229', 'claude-3-haiku-20240307'
  ];
  var MINIMAX_MODELS = [
    'minimax-m2', 'minimax-m2.1', 'minimax-m2.1-lightning', 'abab6.5s-32k', 'abab5.5s-32k'
  ];
  var OPENAI_BASEURL = 'https://api.openai.com';
  var ANTHROPIC_BASEURL = 'https://api.anthropic.com';
  var DEEPSEEK_BASEURL = 'https://api.deepseek.com';
  var MINIMAX_BASEURL_CN = 'https://api.minimaxi.com/anthropic';
  var MINIMAX_BASEURL_INTL = 'https://api.minimax.io/anthropic';

  function getModelList(provider) {
    var p = (provider || '').toLowerCase();
    return p === 'openai' ? OPENAI_MODELS : (p === 'anthropic' ? ANTHROPIC_MODELS : (p === 'minimax' ? MINIMAX_MODELS : []));
  }

  function buildDatalistOptions(values) {
    return (values || []).map(function (v) { return '<option value="' + escapeHtml(v) + '">'; }).join('');
  }

  function setDatalist(el, values) {
    if (!el) return;
    el.innerHTML = buildDatalistOptions(values);
  }

  var inputSelectDropdown = null;
  var inputSelectHideTimer = null;
  var inputSelectJustClosed = false;
  function showInputSelectDropdown(input) {
    if (inputSelectJustClosed) return;
    var listId = input.getAttribute('data-input-select') || input.getAttribute('list');
    if (!listId) return;
    var datalist = document.getElementById(listId);
    if (!datalist) return;
    var opts = Array.from(datalist.querySelectorAll('option')).map(function (o) { return o.getAttribute('value'); }).filter(Boolean);
    if (opts.length === 0) return;
    if (inputSelectHideTimer) { clearTimeout(inputSelectHideTimer); inputSelectHideTimer = null; }
    if (inputSelectDropdown) inputSelectDropdown.remove();
    inputSelectDropdown = document.createElement('div');
    inputSelectDropdown.className = 'input-select-dropdown';
    opts.forEach(function (val) {
      var item = document.createElement('div');
      item.className = 'input-select-dropdown-item';
      item.textContent = val;
      item.addEventListener('mousedown', function (e) {
        e.preventDefault();
        input.value = val;
        inputSelectJustClosed = true;
        setTimeout(function () { inputSelectJustClosed = false; }, 200);
        hideInputSelectDropdown();
      });
      inputSelectDropdown.appendChild(item);
    });
    document.body.appendChild(inputSelectDropdown);
    var rect = input.getBoundingClientRect();
    inputSelectDropdown.style.left = rect.left + 'px';
    inputSelectDropdown.style.top = (rect.bottom + 2) + 'px';
    inputSelectDropdown.style.minWidth = Math.max(rect.width, 120) + 'px';
  }
  function hideInputSelectDropdown() {
    if (inputSelectHideTimer) clearTimeout(inputSelectHideTimer);
    inputSelectHideTimer = setTimeout(function () {
      inputSelectHideTimer = null;
      if (inputSelectDropdown) {
        inputSelectDropdown.remove();
        inputSelectDropdown = null;
      }
    }, 150);
  }
  function hasInputSelect(el) {
    return el && el.tagName === 'INPUT' && (el.getAttribute('data-input-select') || el.getAttribute('list'));
  }
  document.addEventListener('focusin', function (e) {
    if (hasInputSelect(e.target)) showInputSelectDropdown(e.target);
  });
  document.addEventListener('focusout', function (e) {
    if (hasInputSelect(e.target)) hideInputSelectDropdown();
  });

  function appendAgentBlock(container, name, a, providerNames) {
    a = a || {};
    var comp = a.compaction || {};
    var contextWindowK = (comp.contextWindow > 0) ? String(Math.round(comp.contextWindow / 1000)) : '';
    var providerNamesList = providerNames && providerNames.length ? providerNames : ['anthropic', 'openai'];
    var id = 'agent-' + (Date.now().toString(36) + Math.random().toString(36).slice(2));
    var listP = id + '-provider';
    var listM = id + '-model';
    var providerVal = (a.provider || providerNamesList[0] || 'anthropic').trim();
    var modelVal = (a.model || '').trim();
    var modelList = getModelList(providerVal);
    var div = document.createElement('div');
    div.className = 'config-block';
    div.innerHTML = '<div class="config-block-row-head"><label>名称 <input type="text" class="config-agent-name" value="' + escapeHtml(name || '') + '" placeholder="如 default"></label><button type="button" class="config-block-remove">删除</button></div>' +
      '<label>绑定 Provider <input type="text" class="config-agent-provider" data-input-select="' + listP + '" autocomplete="off" value="' + escapeHtml(providerVal) + '" placeholder="如 anthropic"></label><datalist id="' + listP + '">' + buildDatalistOptions(providerNamesList) + '</datalist>' +
      '<label>模型 <input type="text" class="config-agent-model" data-input-select="' + listM + '" autocomplete="off" value="' + escapeHtml(modelVal) + '" placeholder="选或输入模型 ID"></label><datalist id="' + listM + '">' + buildDatalistOptions(modelList) + '</datalist>' +
      '<label>上下文上限 (k) <input type="number" class="config-agent-context-window-k" min="1" step="1" value="' + escapeHtml(contextWindowK) + '" placeholder="如 200"></label>';
    container.appendChild(div);
    div.querySelector('.config-agent-provider').addEventListener('input', function () {
      var listEl = document.getElementById(listM);
      if (listEl) setDatalist(listEl, getModelList(this.value));
    });
    div.querySelector('.config-block-remove').addEventListener('click', function () { div.remove(); });
  }

  function getAgentProvider(block) {
    return (block.querySelector('.config-agent-provider') || {}).value || '';
  }

  function getAgentModelId(block) {
    return (block.querySelector('.config-agent-model') || {}).value || '';
  }

  function getBaseURLDatalistOptions(providerName) {
    var p = (providerName || '').trim().toLowerCase();
    if (p === 'openai') return [OPENAI_BASEURL];
    if (p === 'anthropic') return [ANTHROPIC_BASEURL];
    if (p === 'deepseek') return [DEEPSEEK_BASEURL];
    if (p === 'minimax') return [MINIMAX_BASEURL_CN, MINIMAX_BASEURL_INTL];
    return [];
  }

  function getDefaultBaseURL(providerName) {
    var p = (providerName || '').trim().toLowerCase();
    if (p === 'openai') return OPENAI_BASEURL;
    if (p === 'anthropic') return ANTHROPIC_BASEURL;
    if (p === 'deepseek') return DEEPSEEK_BASEURL;
    if (p === 'minimax') return MINIMAX_BASEURL_CN;
    return '';
  }

  function appendProviderBlock(container, name, p) {
    p = p || {};
    var typeVal = (p.type || '').trim().toLowerCase();
    if (typeVal !== 'openai' && typeVal !== 'anthropic') typeVal = '';
    var nameTrim = (name || '').trim().toLowerCase();
    if (nameTrim === 'minimax' && !typeVal) typeVal = 'anthropic';
    var baseURLVal = (p.baseURL || '').trim();
    if (!baseURLVal) baseURLVal = getDefaultBaseURL(nameTrim);
    var id = 'provider-' + (Date.now().toString(36) + Math.random().toString(36).slice(2));
    var listType = id + '-type';
    var listBase = id + '-baseurl';
    var baseURLOptions = getBaseURLDatalistOptions(nameTrim);
    var div = document.createElement('div');
    div.className = 'config-block';
    div.innerHTML = '<div class="config-block-row-head"><label>名称 <input type="text" class="config-provider-name" value="' + escapeHtml(name || '') + '" placeholder="如 openai"></label><button type="button" class="config-block-remove">删除</button></div>' +
      '<label>API Key <input type="password" class="config-provider-apikey" value="' + escapeHtml(p.apiKey || '') + '" placeholder="必填"></label>' +
      '<label>Base URL <input type="text" class="config-provider-baseurl" data-input-select="' + listBase + '" autocomplete="off" value="' + escapeHtml(baseURLVal) + '" placeholder="可选，常见服务可下拉选或输入"></label><datalist id="' + listBase + '">' + buildDatalistOptions(baseURLOptions) + '</datalist>' +
      '<label>Type <input type="text" class="config-provider-type" data-input-select="' + listType + '" autocomplete="off" value="' + escapeHtml(typeVal) + '" placeholder="自动 或 openai/anthropic"></label><datalist id="' + listType + '"><option value=""><option value="openai"><option value="anthropic"></datalist>';
    container.appendChild(div);
    div.querySelector('.config-provider-name').addEventListener('input', function () {
      var listEl = document.getElementById(listBase);
      if (listEl) {
        var opts = getBaseURLDatalistOptions(this.value);
        setDatalist(listEl, opts);
        var baseInput = div.querySelector('.config-provider-baseurl');
        if (opts.length && !baseInput.value.trim()) baseInput.value = getDefaultBaseURL(this.value) || opts[0];
      }
    });
    div.querySelector('.config-block-remove').addEventListener('click', function () { div.remove(); });
  }

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

  function getCurrentChannel() { return currentChannel; }
  function getCurrentChannelChatId() { return currentChannelChatId; }

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
    closeWs();
    var url = getWsUrl();
    try {
      ws = new WebSocket(url);
      closedByUser = false;
    } catch (e) {
      setStatus('无法创建 WebSocket', 'error');
      return;
    }
    ws.onopen = function () {
      ws.send(JSON.stringify({
        type: 'req',
        id: 'connect',
        method: 'connect',
        params: { role: 'client', token: token }
      }));
    };
    ws.onmessage = function (ev) {
      var msg;
      try {
        msg = JSON.parse(ev.data);
      } catch (e) { return; }
      if (msg.type === 'res' && msg.id === 'connect') {
        if (msg.ok === true) {
          reconnectAttempts = 0;
          currentChannel = 'webchat';
          currentChannelChatId = getDeviceId();
          setStatus('已连接 (WebSocket)', 'connected');
          connectBtn.textContent = '已连接';
          connectBtn.disabled = true;
          try {
            sessionStorage.setItem('aido_token', token);
            localStorage.setItem('aido_token', token);
          } catch (e) {}
          ws.onmessage = onWsMessage;
          loadChatHistory();
          setTimeout(loadChatHistory, 500);
          loadHealth();
          loadSessions();
          loadConfig();
        } else {
          setStatus('认证失败', 'error');
          closeWs();
        }
        return;
      }
    };
    ws.onclose = function () {
      ws = null;
      setStatus('连接已断开', 'error');
      connectBtn.textContent = '连接';
      connectBtn.disabled = false;
      if (closedByUser) {
        closedByUser = false;
        return;
      }
      var t = token || (function () { try { return sessionStorage.getItem('aido_token') || localStorage.getItem('aido_token'); } catch (e) { return ''; } })();
      if (!t) return;
      if (reconnectAttempts >= maxReconnectAttempts) {
        setStatus('连接已断开（已达重试上限）', 'error');
        return;
      }
      reconnectAttempts++;
      var delay = 1000 + (reconnectAttempts - 1) * 500;
      setTimeout(function () {
        tokenEl.value = t;
        token = t;
        connect();
      }, Math.min(delay, 3000));
    };
    ws.onerror = function () {
      setStatus('WebSocket 连接失败', 'error');
    };
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

  var AVATAR_USER = '<span class="msg-avatar msg-avatar-user" aria-hidden="true">我</span>';
  var AVATAR_ASSISTANT = '<span class="msg-avatar msg-avatar-assistant" aria-hidden="true">A</span>';

  function appendMessage(role, text) {
    if (!text) return;
    var div = document.createElement('div');
    div.className = 'msg ' + role;
    var body = role === 'assistant' ? markdownToHtml(text) : escapeHtml(text);
    var avatar = role === 'user' ? AVATAR_USER : AVATAR_ASSISTANT;
    var roleText = role === 'user' ? '我' : 'Aido';
    div.innerHTML = '<div class="msg-head">' + avatar + '<span class="role">' + roleText + '</span></div><div class="msg-body">' + body + '</div>';
    chatHistory.appendChild(div);
    chatHistory.scrollTop = chatHistory.scrollHeight;
  }

  function appendAssistantMessage(text, toolSteps) {
    var div = document.createElement('div');
    div.className = 'msg assistant';
    var parts = ['<div class="msg-head">' + AVATAR_ASSISTANT + '<span class="role">Aido</span></div>'];
    parts.push(buildExecutionLogFromSteps(toolSteps || []));
    parts.push('<div class="msg-body">' + (text ? markdownToHtml(text) : '') + '</div>');
    div.innerHTML = parts.join('');
    chatHistory.appendChild(div);
    chatHistory.scrollTop = chatHistory.scrollHeight;
    applyExecutionLogState();
  }

  function getLastMessageRole(list) {
    if (!list || !list.length) return null;
    for (var idx = list.length - 1; idx >= 0; idx--) {
      var r = list[idx].role;
      if (r === 'user' || r === 'assistant') return r;
    }
    return null;
  }

  function renderChatHistory(list) {
    if (!list || !list.length) return;
    chatHistory.innerHTML = '';
    var i = 0;
    while (i < list.length) {
      var msg = list[i];
      var role = msg.role;
      var content = typeof msg.content === 'string' ? msg.content : (msg.content && msg.content[0] && msg.content[0].text) ? msg.content[0].text : '';
      if (role === 'user') {
        appendMessage('user', content);
        i++;
        continue;
      }
      if (role === 'assistant' && (msg.tool_calls || msg.toolCalls) && (msg.tool_calls || msg.toolCalls).length > 0) {
        var allSteps = [];
        var finalContent = '';
        var j = i;
        while (j < list.length) {
          var cur = list[j];
          if (cur.role !== 'assistant') break;
          var hasCalls = (cur.tool_calls || cur.toolCalls) && (cur.tool_calls || cur.toolCalls).length > 0;
          if (!hasCalls) {
            finalContent = typeof cur.content === 'string' ? cur.content : (cur.content && cur.content[0] && cur.content[0].text) ? cur.content[0].text : '';
            j++;
            break;
          }
          var calls = cur.tool_calls || cur.toolCalls;
          var k = j + 1;
          while (k < list.length && list[k].role === 'tool') {
            var toolMsg = list[k];
            var stepIdx = k - (j + 1);
            var call = stepIdx < calls.length ? calls[stepIdx] : null;
            if (call) {
              allSteps.push({
                toolName: call.name || call.toolName || '',
                toolParams: (call.arguments != null ? call.arguments : call.toolParams) || '',
                toolResult: typeof toolMsg.content === 'string' ? toolMsg.content : ''
              });
            }
            k++;
          }
          j = k;
        }
        appendAssistantMessage(finalContent, allSteps.length ? allSteps : null);
        i = j;
        continue;
      }
      if (role === 'assistant') {
        appendMessage('assistant', content);
        i++;
        continue;
      }
      if (role === 'tool') { i++; continue; }
      i++;
    }
    chatHistory.scrollTop = chatHistory.scrollHeight;
  }

  function stopHistoryPolling() {
    if (historyPollTimer) {
      clearTimeout(historyPollTimer);
      historyPollTimer = null;
    }
    if (historyPollPlaceholder && historyPollPlaceholder.parentNode) {
      historyPollPlaceholder.remove();
      historyPollPlaceholder = null;
    }
  }

  function loadChatHistory() {
    if (!currentChannel || !currentChannelChatId) return;
    var p = ws && ws.readyState === 1
      ? wsRequest('chat.history', { channel: currentChannel, channelChatId: currentChannelChatId })
      : apiCall('/chat/history?channel=' + encodeURIComponent(currentChannel) + '&channelChatId=' + encodeURIComponent(currentChannelChatId));
    p.then(function (res) {
      if (!res || !res.messages) return;
      stopHistoryPolling();
      renderChatHistory(res.messages);
      applyExecutionLogState();
      if (getLastMessageRole(res.messages) !== 'user') return;
      historyPollPlaceholder = document.createElement('div');
      historyPollPlaceholder.className = 'msg assistant';
      historyPollPlaceholder.innerHTML = '<div class="msg-head">' + AVATAR_ASSISTANT + '<span class="role">Aido</span></div><details class="execution-log"><summary>执行过程</summary><div class="execution-log-inner"><div class="execution-log-line execution-log-status">正在获取最新回复…（无需刷新）</div></div></details>';
      chatHistory.appendChild(historyPollPlaceholder);
      applyExecutionLogState();
      chatHistory.scrollTop = chatHistory.scrollHeight;
      var pollStart = Date.now();
      var maxPollMs = 5 * 60 * 1000;
      var pollCount = 0;
      var fastPollRounds = 3;
      function doPoll() {
        if (!historyPollPlaceholder || !historyPollPlaceholder.parentNode) return;
        if (Date.now() - pollStart > maxPollMs) {
          stopHistoryPolling();
          return;
        }
        var q = ws && ws.readyState === 1
          ? wsRequest('chat.history', { channel: currentChannel, channelChatId: currentChannelChatId })
          : apiCall('/chat/history?channel=' + encodeURIComponent(currentChannel) + '&channelChatId=' + encodeURIComponent(currentChannelChatId));
        q.then(function (nextRes) {
          if (!nextRes || !nextRes.messages) return;
          if (getLastMessageRole(nextRes.messages) === 'assistant') {
            stopHistoryPolling();
            renderChatHistory(nextRes.messages);
            applyExecutionLogState();
          }
        });
        pollCount++;
        var delay = pollCount < fastPollRounds ? 1000 : 2000;
        historyPollTimer = setTimeout(doPoll, delay);
      }
      historyPollTimer = setTimeout(doPoll, 1000);
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
    if (!ws || ws.readyState !== 1) return;
    stopHistoryPolling();
    chatInput.value = '';
    var streamDiv = null;
    agentEventCallback = function (ev) {
        if (ev.type === 'stream_start' && !streamDiv) {
          streamDiv = document.createElement('div');
          streamDiv.className = 'msg assistant streaming';
          streamDiv.innerHTML = '<div class="msg-head">' + AVATAR_ASSISTANT + '<span class="role">Aido</span></div><details class="execution-log"><summary>执行过程</summary><div class="execution-log-inner"></div></details><div class="msg-body"></div>';
          chatHistory.appendChild(streamDiv);
          applyExecutionLogState();
          chatHistory.scrollTop = chatHistory.scrollHeight;
        }
        if (!streamDiv) return;
        var logEl = streamDiv.querySelector('.execution-log-inner');
        var body = streamDiv.querySelector('.msg-body');
        if (ev.type === 'stream_start' && logEl) {
          appendExecutionLog(logEl, 'status', escapeHtml(EXEC.start));
          chatHistory.scrollTop = chatHistory.scrollHeight;
        } else if (ev.type === 'tool_start' && logEl) {
          var params = (ev.toolParams || '').trim();
          var short = params.length > 80 ? params.slice(0, 80) + '…' : params;
          appendExecutionLog(logEl, 'tool-start', EXEC.call + escapeHtml(ev.toolName || 'tool') + (short ? ': ' + escapeHtml(short) : ''));
          chatHistory.scrollTop = chatHistory.scrollHeight;
        } else if (ev.type === 'tool_end' && logEl) {
          var res = (ev.toolResult || '').trim();
          var resShort = res.length > 120 ? res.slice(0, 120) + '…' : res;
          appendExecutionLog(logEl, 'tool-end', EXEC.return + (resShort ? escapeHtml(resShort) : '—'));
          chatHistory.scrollTop = chatHistory.scrollHeight;
        } else if (ev.type === 'text_delta' && ev.text && body) {
          body.innerHTML += escapeHtml(ev.text);
          chatHistory.scrollTop = chatHistory.scrollHeight;
        } else if (ev.type === 'done' && logEl) {
          appendExecutionLog(logEl, 'status', escapeHtml(EXEC.done));
          removeExecutionLogIfEmpty(logEl);
          chatHistory.scrollTop = chatHistory.scrollHeight;
        } else if (ev.type === 'error' && ev.error && logEl) {
          appendExecutionLog(logEl, 'error', EXEC.error + escapeHtml(ev.error));
          chatHistory.scrollTop = chatHistory.scrollHeight;
        }
      };
      wsRequest('message.send', { channel: currentChannel, channelChatId: currentChannelChatId, text: text }, 120000).then(function (res) {
        agentEventCallback = null;
        if (streamDiv) {
          streamDiv.classList.remove('streaming');
          var body = streamDiv.querySelector('.msg-body');
          if (body) body.innerHTML = res.text ? markdownToHtml(res.text) : '';
        } else {
          appendAssistantMessage(res.text || '', res.toolSteps);
        }
        chatHistory.scrollTop = chatHistory.scrollHeight;
      }).catch(function (err) {
        agentEventCallback = null;
        if (streamDiv) streamDiv.remove();
        appendMessage('assistant', '发送失败: ' + (err && err.message ? err.message : '未知错误'));
      });
  });

  var EXEC = {
    start: '开始执行…',
    call: '调用 ',
    return: '→ 返回 ',
    done: '完成',
    error: '错误: '
  };

  function appendExecutionLog(container, kind, html) {
    var line = document.createElement('div');
    line.className = 'execution-log-line execution-log-' + (kind || 'status');
    line.innerHTML = html;
    container.appendChild(line);
  }

  function buildExecutionLogFromSteps(toolSteps) {
    var steps = toolSteps || [];
    if (steps.length === 0) return '';
    var lines = [];
    lines.push('<div class="execution-log-line execution-log-status">' + escapeHtml(EXEC.start) + '</div>');
    steps.forEach(function (step) {
      var params = (step.toolParams || '').trim();
      var result = (step.toolResult || '').trim();
      var paramsShort = params.length > 80 ? params.slice(0, 80) + '…' : params;
      var resultShort = result.length > 120 ? result.slice(0, 120) + '…' : result;
      lines.push('<div class="execution-log-line execution-log-tool-start">' + EXEC.call + escapeHtml(step.toolName || 'tool') + (paramsShort ? ': ' + escapeHtml(paramsShort) : '') + '</div>');
      lines.push('<div class="execution-log-line execution-log-tool-end">' + EXEC.return + (resultShort ? escapeHtml(resultShort) : '—') + '</div>');
    });
    lines.push('<div class="execution-log-line execution-log-status">' + escapeHtml(EXEC.done) + '</div>');
    return '<details class="execution-log"><summary>执行过程</summary><div class="execution-log-inner">' + lines.join('') + '</div></details>';
  }

  function removeExecutionLogIfEmpty(logEl) {
    if (!logEl || logEl.querySelectorAll('.execution-log-tool-start').length > 0) return;
    var details = logEl.closest('details.execution-log');
    if (details) details.remove();
  }

  chatInput.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendBtn.click();
    }
  });

  var EXECUTION_LOG_EXPANDED_KEY = 'aido_execution_log_expanded';

  function applyExecutionLogState() {
    if (!chatHistory) return;
    var open = executionLogSwitch ? executionLogSwitch.checked : (localStorage.getItem(EXECUTION_LOG_EXPANDED_KEY) === 'true');
    chatHistory.querySelectorAll('details.execution-log').forEach(function (el) {
      if (open) el.setAttribute('open', ''); else el.removeAttribute('open');
    });
  }

  if (executionLogSwitch) {
    try {
      var saved = localStorage.getItem(EXECUTION_LOG_EXPANDED_KEY);
      executionLogSwitch.checked = saved === 'true';
    } catch (e) {}
    applyExecutionLogState();
    executionLogSwitch.addEventListener('change', function () {
      try {
        localStorage.setItem(EXECUTION_LOG_EXPANDED_KEY, this.checked ? 'true' : 'false');
      } catch (e) {}
      applyExecutionLogState();
    });
  }

  function loadSessions() {
    if (!token) {
      sessionsList.innerHTML = '<div class="session-item">请先连接</div>';
      return;
    }
    var p = ws && ws.readyState === 1
      ? wsRequest('sessions.list', {})
      : apiCall('/sessions');
    p.then(function (res) {
      if (!res || !res.sessions) {
        sessionsList.innerHTML = '<div class="session-item">暂无会话或未连接</div>';
        return;
      }
      sessionsList.innerHTML = res.sessions.map(function (s) {
        var label = (s.channel || '') + ':' + (s.channelChatId || '');
        return '<div class="session-item"><span class="session-key">' + escapeHtml(label) + '</span><div class="session-meta">' +
          '更新: ' + (s.updatedAt || '') + '</div></div>';
      }).join('') || '<div class="session-item">暂无会话</div>';
    });
  }

  refreshSessions.addEventListener('click', loadSessions);

  function loadHealth() {
    if (!token) {
      healthInfo.textContent = '请先连接';
      return;
    }
    var p = ws && ws.readyState === 1
      ? wsRequest('health', {})
      : apiCall('/health');
    p.then(function (res) {
      healthInfo.textContent = res ? JSON.stringify(res, null, 2) : '获取失败';
    });
  }

  function showConfigMessage(text, isError) {
    configMessage.textContent = text || '';
    configMessage.className = 'config-message' + (isError ? ' config-message-error' : ' config-message-success');
    configMessage.style.display = text ? 'block' : 'none';
  }

  function fillConfigForm(res) {
    if (!res) return;
    currentConfig = res;
    configPathDisplay.textContent = res.configPath || '—';
    if (res.gateway) {
      configGatewayPort.value = res.gateway.port != null ? res.gateway.port : '';
      configGatewayToken.value = (res.gateway.auth && res.gateway.auth.token) || '';
      if (configGatewayToolsProfile) {
        var gp = (res.gateway.toolsProfile || 'coding').toLowerCase();
        configGatewayToolsProfile.value = ['minimal', 'coding', 'messaging', 'full'].indexOf(gp) >= 0 ? gp : 'coding';
      }
      if (configGatewayLocale) {
        var loc = (res.gateway.locale || 'zh').toLowerCase();
        configGatewayLocale.value = (loc === 'en' ? 'en' : 'zh');
      }
    }
    var gatewayAgentList = document.getElementById('gateway-current-agent-list');
    if (gatewayAgentList && res.agents && typeof res.agents === 'object') {
      setDatalist(gatewayAgentList, Object.keys(res.agents));
    }
    var providerNames = (res.providers && typeof res.providers === 'object') ? Object.keys(res.providers) : [];
    if (providerNames.length === 0) providerNames = ['anthropic', 'openai'];
    configAgents.innerHTML = '';
    if (res.agents && typeof res.agents === 'object') {
      Object.keys(res.agents).forEach(function (name) {
        appendAgentBlock(configAgents, name, res.agents[name], providerNames);
      });
      if (configGatewayCurrentAgent) configGatewayCurrentAgent.value = (res.gateway && res.gateway.currentAgent) || '';
    }
    configProviders.innerHTML = '';
    if (res.providers && typeof res.providers === 'object') {
      Object.keys(res.providers).forEach(function (name) {
        appendProviderBlock(configProviders, name, res.providers[name]);
      });
    }
    configMCP.innerHTML = '';
    var mcpList = (res.tools && res.tools.mcp) || [];
    mcpList.forEach(function (m) {
      appendMCPRow(m.name || '', m.command || '', (m.args || []).join(', '), m.url || '', m.transport || '', m.env || {});
    });
    if (configBridges) {
      configBridges.innerHTML = '';
      var bridgeList = (res.bridges && res.bridges.instances) || [];
      bridgeList.forEach(function (b) {
        appendBridgeRow(b.id || '', b.enabled !== false, b.path || '', b.env || {});
      });
    }
  }

  function appendMCPEnvRow(container, key, value) {
    var row = document.createElement('div');
    row.className = 'config-mcp-env-row config-block-row-inline';
    row.innerHTML = '<label>变量名 <input type="text" class="config-mcp-env-key" value="' + escapeHtml(key || '') + '" placeholder="如 GITHUB_TOKEN"></label>' +
      '<label>值 <input type="text" class="config-mcp-env-val" value="' + escapeHtml(value || '') + '" placeholder="可选"></label>' +
      '<button type="button" class="config-block-remove-inline">删除</button>';
    container.appendChild(row);
    row.querySelector('.config-block-remove-inline').addEventListener('click', function () { row.remove(); });
  }

  function appendBridgeEnvRow(container, key, value) {
    var row = document.createElement('div');
    row.className = 'config-bridge-env-row config-block-row-inline';
    row.innerHTML = '<label>变量名 <input type="text" class="config-bridge-env-key" value="' + escapeHtml(key || '') + '"></label>' +
      '<label>值 <input type="text" class="config-bridge-env-val" value="' + escapeHtml(value || '') + '"></label>' +
      '<button type="button" class="config-block-remove-inline">删除</button>';
    container.appendChild(row);
    row.querySelector('.config-block-remove-inline').addEventListener('click', function () { row.remove(); });
  }
  function appendBridgeRow(id, enabled, path, env) {
    if (!configBridges) return;
    var envObj = typeof env === 'object' && env !== null ? env : {};
    var div = document.createElement('div');
    div.className = 'config-block config-block-row';
    div.innerHTML = '<div class="config-block-row-head"><span class="config-block-title">Bridge</span><button type="button" class="config-block-remove">删除</button></div>' +
      '<label>ID <input type="text" class="config-bridge-id" value="' + escapeHtml(id) + '" placeholder="如 feishu"></label>' +
      '<label class="execution-log-switch"><input type="checkbox" class="config-bridge-enabled" ' + (enabled ? 'checked' : '') + '><span class="execution-log-switch-slider"></span><span class="execution-log-switch-label">启用</span></label>' +
      '<label>路径 <input type="text" class="config-bridge-path" value="' + escapeHtml(path) + '" placeholder="bridges/feishu 或绝对路径"></label>' +
      '<div class="config-mcp-env-block"><span class="config-block-label">Env</span><div class="config-bridge-env-list"></div><button type="button" class="config-add-btn-inline config-bridge-env-add">添加变量</button></div>';
    configBridges.appendChild(div);
    var envList = div.querySelector('.config-bridge-env-list');
    Object.keys(envObj).forEach(function (k) { appendBridgeEnvRow(envList, k, envObj[k]); });
    if (envList.children.length === 0) appendBridgeEnvRow(envList, '', '');
    div.querySelector('.config-bridge-env-add').addEventListener('click', function () { appendBridgeEnvRow(envList, '', ''); });
    div.querySelector('.config-block-remove').addEventListener('click', function () { div.remove(); });
  }
  function appendMCPRow(name, command, argsStr, url, transport, env) {
    var envObj = typeof env === 'object' && env !== null ? env : {};
    var transportVal = (transport || 'stdio').toLowerCase();
    if (transportVal !== 'stdio' && transportVal !== 'http') transportVal = 'stdio';
    var mcpId = 'mcp-' + (Date.now().toString(36) + Math.random().toString(36).slice(2));
    var div = document.createElement('div');
    div.className = 'config-block config-block-row';
    div.innerHTML = '<div class="config-block-row-head"><span class="config-block-title">MCP 服务</span><button type="button" class="config-block-remove">删除</button></div>' +
      '<label>名称 <input type="text" class="config-mcp-name" value="' + escapeHtml(name) + '" placeholder="mcp"></label>' +
      '<label>Transport <input type="text" class="config-mcp-transport" data-input-select="' + mcpId + '-transport" autocomplete="off" value="' + escapeHtml(transportVal) + '" placeholder="stdio 或 http"></label><datalist id="' + mcpId + '-transport"><option value="stdio"><option value="http"></datalist>' +
      '<label>Command <input type="text" class="config-mcp-command" value="' + escapeHtml(command) + '" placeholder="npx"></label>' +
      '<label>Args (逗号分隔) <input type="text" class="config-mcp-args" value="' + escapeHtml(argsStr) + '" placeholder="e.g. -y, @model/server"></label>' +
      '<label>URL <input type="text" class="config-mcp-url" value="' + escapeHtml(url) + '" placeholder="HTTP 时填 SSE 地址"></label>' +
      '<div class="config-mcp-env-block"><span class="config-block-label">Env（键值对）</span><div class="config-mcp-env-list"></div><button type="button" class="config-add-btn-inline config-mcp-env-add">添加变量</button></div>';
    configMCP.appendChild(div);
    var envList = div.querySelector('.config-mcp-env-list');
    Object.keys(envObj).forEach(function (k) { appendMCPEnvRow(envList, k, envObj[k]); });
    if (envList.children.length === 0) appendMCPEnvRow(envList, '', '');
    div.querySelector('.config-mcp-env-add').addEventListener('click', function () { appendMCPEnvRow(envList, '', ''); });
    div.querySelector('.config-block-remove').addEventListener('click', function () { div.remove(); });
  }

  function collectConfigFromForm() {
    var cfg = {
      gateway: {
        port: parseInt(configGatewayPort.value, 10) || 19800,
        currentAgent: (configGatewayCurrentAgent && configGatewayCurrentAgent.value) ? configGatewayCurrentAgent.value.trim() : '',
        toolsProfile: (configGatewayToolsProfile && configGatewayToolsProfile.value) ? configGatewayToolsProfile.value.trim() : 'coding',
        locale: (configGatewayLocale && configGatewayLocale.value) ? configGatewayLocale.value : 'zh',
        auth: {
          token: configGatewayToken.value.trim()
        }
      },
      agents: {},
      providers: {},
      tools: {},
      bridges: { instances: [] }
    };
    configAgents.querySelectorAll('.config-block').forEach(function (block) {
      var name = (block.querySelector('.config-agent-name') || {}).value;
      if (!name || !name.trim()) return;
      name = name.trim();
      var provider = getAgentProvider(block);
      var modelId = getAgentModelId(block);
      var base = (currentConfig && currentConfig.agents && currentConfig.agents[name]) || {};
      var comp = Object.assign({}, base.compaction || {});
      comp.contextWindow = 0;
      var ctxKEl = block.querySelector('.config-agent-context-window-k');
      if (ctxKEl && ctxKEl.value.trim() !== '') {
        var k = parseInt(ctxKEl.value.trim(), 10);
        if (!isNaN(k) && k > 0) comp.contextWindow = k * 1000;
      }
      cfg.agents[name] = {
        provider: provider,
        model: modelId,
        tools: { allow: base.tools && base.tools.allow, deny: base.tools && base.tools.deny },
        fallbacks: base.fallbacks,
        compaction: comp,
        workspace: base.workspace,
        skills: base.skills
      };
    });
    configProviders.querySelectorAll('.config-block').forEach(function (block) {
      var name = (block.querySelector('.config-provider-name') || {}).value;
      if (!name || !name.trim()) return;
      name = name.trim();
      var apiKey = (block.querySelector('.config-provider-apikey') || {}).value || '';
      var baseURL = (block.querySelector('.config-provider-baseurl') || {}).value || '';
      var typeVal = (block.querySelector('.config-provider-type') || {}).value || '';
      cfg.providers[name] = {
        apiKey: apiKey,
        baseURL: baseURL.trim(),
        type: typeVal.trim()
      };
    });
    cfg.tools = { mcp: [] };
    configMCP.querySelectorAll('.config-block-row').forEach(function (block) {
      var name = (block.querySelector('.config-mcp-name') || {}).value || '';
      var command = (block.querySelector('.config-mcp-command') || {}).value || '';
      var argsStr = (block.querySelector('.config-mcp-args') || {}).value || '';
      var url = (block.querySelector('.config-mcp-url') || {}).value || '';
      var transport = (block.querySelector('.config-mcp-transport') || {}).value || '';
      var env = {};
      (block.querySelectorAll('.config-mcp-env-row') || []).forEach(function (row) {
        var k = (row.querySelector('.config-mcp-env-key') || {}).value;
        var v = (row.querySelector('.config-mcp-env-val') || {}).value;
        if (k && k.trim()) env[k.trim()] = v;
      });
      var args = argsStr.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
      if (name || command || url) cfg.tools.mcp.push({ name: name, command: command, args: args, url: url, transport: transport || 'stdio', env: env });
    });
    if (configBridges) {
      configBridges.querySelectorAll('.config-block-row').forEach(function (block) {
        var id = (block.querySelector('.config-bridge-id') || {}).value || '';
        var enabled = (block.querySelector('.config-bridge-enabled') || {}).checked !== false;
        var path = (block.querySelector('.config-bridge-path') || {}).value || '';
        var env = {};
        (block.querySelectorAll('.config-bridge-env-list .config-bridge-env-row') || []).forEach(function (row) {
          var k = (row.querySelector('.config-bridge-env-key') || {}).value || '';
          var v = (row.querySelector('.config-bridge-env-val') || {}).value || '';
          if (k && k.trim()) env[k.trim()] = v;
        });
        if (id || path) cfg.bridges.instances.push({ id: id.trim() || 'bridge', enabled: enabled, path: path.trim(), env: env });
      });
    }
    return cfg;
  }

  function loadConfig() {
    if (!token) {
      showConfigMessage('请先连接', true);
      configPathDisplay.textContent = '—';
      return;
    }
    var p = ws && ws.readyState === 1
      ? wsRequest('config.get', {})
      : apiCall('/config');
    p.then(function (res) {
      if (res) fillConfigForm(res);
      else showConfigMessage('获取配置失败', true);
    });
  }

  refreshConfig.addEventListener('click', loadConfig);

  if (configAgentAdd) {
    configAgentAdd.addEventListener('click', function () {
      var providerNames = (currentConfig && currentConfig.providers && typeof currentConfig.providers === 'object') ? Object.keys(currentConfig.providers) : [];
      if (providerNames.length === 0) providerNames = ['anthropic', 'openai'];
      var defaultAgent = { provider: providerNames[0] || 'anthropic', model: '', tools: {} };
      appendAgentBlock(configAgents, '', defaultAgent, providerNames);
    });
  }
  if (configProviderAdd) {
    configProviderAdd.addEventListener('click', function () {
      appendProviderBlock(configProviders, '', {});
    });
  }

  configMCPAdd.addEventListener('click', function () { appendMCPRow('', '', '', '', '', {}); });
  if (configBridgeAdd) configBridgeAdd.addEventListener('click', function () { appendBridgeRow('', true, '', {}); });

  saveConfig.addEventListener('click', function () {
    if (!token) {
      showConfigMessage('请先连接', true);
      return;
    }
    var body = collectConfigFromForm();
    if (!body) return;
    showConfigMessage('保存中…');
    var opts = { method: 'PUT', headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token }, body: JSON.stringify(body) };
    fetch('/api/config', opts).then(function (r) {
      return r.json().then(function (data) {
        return { ok: r.ok, status: r.status, data: data };
      }).catch(function () {
        return { ok: r.ok, status: r.status, data: {} };
      });
    }).then(function (result) {
      if (result.ok) {
        showConfigMessage(result.data.message || '已保存', false);
        loadConfig();
      } else {
        showConfigMessage((result.data && result.data.error) || '保存失败', true);
      }
    }).catch(function () {
      showConfigMessage('网络错误', true);
    });
  });

  function updateConfigNavActive() {
    var hash = (location.hash || '').slice(1);
    document.querySelectorAll('.config-nav-item').forEach(function (a) {
      var h = (a.getAttribute('href') || '').slice(1);
      a.classList.toggle('active', h && h === hash);
    });
  }
  window.addEventListener('hashchange', updateConfigNavActive);
  document.querySelectorAll('.tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
      document.querySelectorAll('.tab').forEach(function (t) { t.classList.remove('active'); });
      document.querySelectorAll('.panel').forEach(function (p) { p.classList.remove('active'); });
      tab.classList.add('active');
      document.getElementById('panel-' + tab.dataset.tab).classList.add('active');
      if (tab.dataset.tab === 'health') loadHealth();
      if (tab.dataset.tab === 'config') {
        loadConfig();
        updateConfigNavActive();
      }
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
