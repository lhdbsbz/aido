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
  var saveConfig = document.getElementById('saveConfig');
  var configMessage = document.getElementById('configMessage');
  var configPathDisplay = document.getElementById('configPathDisplay');
  var configGatewayPort = document.getElementById('configGatewayPort');
  var configGatewayToken = document.getElementById('configGatewayToken');
  var configGatewayCurrentAgent = document.getElementById('configGatewayCurrentAgent');
  var configGatewayToolsProfile = document.getElementById('configGatewayToolsProfile');
  var configAgents = document.getElementById('configAgents');
  var configAgentAdd = document.getElementById('configAgentAdd');
  var configProviders = document.getElementById('configProviders');
  var configProviderAdd = document.getElementById('configProviderAdd');
  var configMCP = document.getElementById('configMCP');
  var configMCPAdd = document.getElementById('configMCPAdd');
  var healthInfo = document.getElementById('healthInfo');

  var currentConfig = null;

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
    var providerNamesList = providerNames && providerNames.length ? providerNames : ['anthropic', 'openai'];
    var id = 'agent-' + (Date.now().toString(36) + Math.random().toString(36).slice(2));
    var listP = id + '-provider';
    var listM = id + '-model';
    var listT = id + '-thinking';
    var providerVal = (a.provider || providerNamesList[0] || 'anthropic').trim();
    var modelVal = (a.model || '').trim();
    var thinkingVal = (a.thinking || '').trim();
    var modelList = getModelList(providerVal);
    var div = document.createElement('div');
    div.className = 'config-block';
    div.innerHTML = '<div class="config-block-row-head"><label>名称 <input type="text" class="config-agent-name" value="' + escapeHtml(name || '') + '" placeholder="如 default"></label><button type="button" class="config-block-remove">删除</button></div>' +
      '<label>绑定 Provider <input type="text" class="config-agent-provider" data-input-select="' + listP + '" autocomplete="off" value="' + escapeHtml(providerVal) + '" placeholder="如 anthropic"></label><datalist id="' + listP + '">' + buildDatalistOptions(providerNamesList) + '</datalist>' +
      '<label>模型 <input type="text" class="config-agent-model" data-input-select="' + listM + '" autocomplete="off" value="' + escapeHtml(modelVal) + '" placeholder="选或输入模型 ID"></label><datalist id="' + listM + '">' + buildDatalistOptions(modelList) + '</datalist>' +
      '<label>Thinking <input type="text" class="config-agent-thinking" data-input-select="' + listT + '" autocomplete="off" value="' + escapeHtml(['off', 'low', 'medium', 'high'].indexOf(thinkingVal) >= 0 ? thinkingVal : '') + '" placeholder="— 或 off/low/medium/high"></label><datalist id="' + listT + '"><option value=""><option value="off"><option value="low"><option value="medium"><option value="high"></datalist>';
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
        auth: {
          token: configGatewayToken.value.trim()
        }
      },
      agents: {},
      providers: {},
      tools: {}
    };
    configAgents.querySelectorAll('.config-block').forEach(function (block) {
      var name = (block.querySelector('.config-agent-name') || {}).value;
      if (!name || !name.trim()) return;
      name = name.trim();
      var provider = getAgentProvider(block);
      var modelId = getAgentModelId(block);
      var thinking = (block.querySelector('.config-agent-thinking') || {}).value || '';
      var base = (currentConfig && currentConfig.agents && currentConfig.agents[name]) || {};
      cfg.agents[name] = {
        provider: provider,
        model: modelId,
        thinking: thinking,
        tools: { allow: base.tools && base.tools.allow, deny: base.tools && base.tools.deny },
        fallbacks: base.fallbacks,
        compaction: base.compaction,
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
    return cfg;
  }

  function loadConfig() {
    if (!token) {
      showConfigMessage('请先连接', true);
      configPathDisplay.textContent = '—';
      return;
    }
    showConfigMessage('');
    apiCall('/config').then(function (res) {
      if (res) fillConfigForm(res);
      else showConfigMessage('获取配置失败', true);
    });
  }

  refreshConfig.addEventListener('click', loadConfig);

  if (configAgentAdd) {
    configAgentAdd.addEventListener('click', function () {
      var providerNames = (currentConfig && currentConfig.providers && typeof currentConfig.providers === 'object') ? Object.keys(currentConfig.providers) : [];
      if (providerNames.length === 0) providerNames = ['anthropic', 'openai'];
      var defaultAgent = { provider: providerNames[0] || 'anthropic', model: '', thinking: 'medium', tools: {} };
      appendAgentBlock(configAgents, '', defaultAgent, providerNames);
    });
  }
  if (configProviderAdd) {
    configProviderAdd.addEventListener('click', function () {
      appendProviderBlock(configProviders, '', {});
    });
  }

  configMCPAdd.addEventListener('click', function () { appendMCPRow('', '', '', '', '', {}); });

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
