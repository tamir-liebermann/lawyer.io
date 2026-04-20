// Frontend chat logic for lawyer.io MVP.
// All user-facing strings are Hebrew; all code comments are English.
(function () {
  'use strict';

  const els = {
    messages: document.getElementById('messages'),
    form: document.getElementById('chatForm'),
    input: document.getElementById('input'),
    sendBtn: document.getElementById('sendBtn'),
    resetBtn: document.getElementById('resetBtn'),
    chips: document.getElementById('suggestedChips'),
    quickTools: document.getElementById('quickTools'),
    modeBtns: Array.from(document.querySelectorAll('.mode-btn')),
  };

  const state = {
    mode: 'client', // client | lawyer
    sessionID: null,
    sending: false,
  };

  // ---------- URL param ?mode=lawyer ----------
  const params = new URLSearchParams(window.location.search);
  const urlMode = params.get('mode');
  if (urlMode === 'lawyer' || urlMode === 'client') {
    state.mode = urlMode;
    updateModeButtons();
  }

  // ---------- Rendering ----------
  function addMessage(role, text, opts) {
    const wrap = document.createElement('div');
    wrap.className = 'msg ' + (role === 'user' ? 'user' : 'ai');
    if (opts && opts.thinking) wrap.classList.add('thinking');
    const bubble = document.createElement('div');
    bubble.className = 'bubble';
    bubble.textContent = text;
    wrap.appendChild(bubble);
    els.messages.appendChild(wrap);
    els.messages.scrollTop = els.messages.scrollHeight;
    return wrap;
  }

  function renderChips(actions) {
    els.chips.innerHTML = '';
    (actions || []).forEach((a) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'chip';
      b.textContent = a;
      b.addEventListener('click', () => {
        els.input.value = a;
        els.input.focus();
      });
      els.chips.appendChild(b);
    });
  }

  function renderQuickTools() {
    const clientTools = [
      'מה להביא לפגישה ראשונה',
      'שלבי עסקת נדל"ן',
      'מס שבח — הסבר פשוט',
    ];
    const lawyerTools = [
      'איסוף נתונים לטופס 7002',
      'איסוף נתונים לטופס 7000',
      'חיפוש עסקאות בתל אביב',
      'חוק המקרקעין — סעיפים מרכזיים',
    ];
    const list = state.mode === 'lawyer' ? lawyerTools : clientTools;
    els.quickTools.innerHTML = '';
    list.forEach((t) => {
      const li = document.createElement('li');
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = t;
      b.addEventListener('click', () => {
        els.input.value = t;
        els.input.focus();
      });
      li.appendChild(b);
      els.quickTools.appendChild(li);
    });
  }

  function updateModeButtons() {
    els.modeBtns.forEach((btn) => {
      const active = btn.dataset.mode === state.mode;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    renderQuickTools();
  }

  // ---------- Networking ----------
  async function sendChat(message) {
    const res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ message: message, mode: state.mode }),
    });
    const text = await res.text();
    let data;
    try { data = JSON.parse(text); } catch (e) { data = { error: text }; }
    if (!res.ok) {
      const msg = (data && data.error) || 'שגיאה לא צפויה';
      throw new Error(msg);
    }
    return data;
  }

  async function setModeOnServer(mode) {
    try {
      await fetch('/api/mode', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ mode: mode }),
      });
    } catch (_) { /* non-fatal; /api/chat will also persist */ }
  }

  async function resetChat() {
    try {
      await fetch('/api/reset', {
        method: 'POST',
        credentials: 'same-origin',
      });
    } catch (_) {}
    els.messages.innerHTML = '';
    addMessage('ai', 'שיחה חדשה. איך אפשר לעזור?');
  }

  // ---------- Handlers ----------
  els.modeBtns.forEach((btn) => {
    btn.addEventListener('click', () => {
      state.mode = btn.dataset.mode;
      updateModeButtons();
      setModeOnServer(state.mode);
    });
  });

  els.form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (state.sending) return;
    const msg = els.input.value.trim();
    if (!msg) return;

    addMessage('user', msg);
    els.input.value = '';
    state.sending = true;
    els.sendBtn.disabled = true;
    const thinking = addMessage('ai', 'חושב', { thinking: true });

    try {
      const data = await sendChat(msg);
      thinking.remove();
      state.sessionID = data.session_id || state.sessionID;
      if (data.user_type && data.user_type !== state.mode) {
        state.mode = data.user_type;
        updateModeButtons();
      }
      addMessage('ai', data.reply || '(תגובה ריקה)');
      renderChips(data.suggested_actions);
    } catch (err) {
      thinking.remove();
      addMessage('ai', 'שגיאה: ' + err.message);
    } finally {
      state.sending = false;
      els.sendBtn.disabled = false;
      els.input.focus();
    }
  });

  // Enter to send, Shift+Enter for newline.
  els.input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      els.form.requestSubmit();
    }
  });

  // Auto-grow textarea.
  els.input.addEventListener('input', () => {
    els.input.style.height = 'auto';
    els.input.style.height = Math.min(els.input.scrollHeight, 180) + 'px';
  });

  els.resetBtn.addEventListener('click', resetChat);

  // Init
  renderQuickTools();
  renderChips([
    'מה להביא לפגישה ראשונה',
    'כמה זמן לוקח רישום בטאבו?',
    'מה זה מס שבח?',
  ]);
})();
