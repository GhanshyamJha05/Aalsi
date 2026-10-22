// Aalsi Web Application Logic - Material You (MD3) Edition
let currentLang = 'en';
let translations = {};

// DOM Elements
const dropzone = document.getElementById('dropzone');
const fileInput = document.getElementById('fileInput');
const browseBtn = document.getElementById('browseBtn');
const uploadProgress = document.getElementById('uploadProgress');
const digestSummary = document.getElementById('digestSummary');
const extractedByMode = document.getElementById('extractedByMode');
const todayList = document.getElementById('todayList');
const tomorrowList = document.getElementById('tomorrowList');
const weekList = document.getElementById('weekList');
const missedList = document.getElementById('missedList');
const reviewList = document.getElementById('reviewList');
const todayCount = document.getElementById('todayCount');
const tomorrowCount = document.getElementById('tomorrowCount');
const weekCount = document.getElementById('weekCount');
const missedCount = document.getElementById('missedCount');
const reviewCount = document.getElementById('reviewCount');
const langEnBtn = document.getElementById('langEnBtn');
const langHiBtn = document.getElementById('langHiBtn');
const copyDigestBtn = document.getElementById('copyDigestBtn');
const settingsBtn = document.getElementById('settingsBtn');
const settingsModal = document.getElementById('settingsModal');
const closeSettingsBtn = document.getElementById('closeSettingsBtn');
const modelSelect = document.getElementById('modelSelect');
const modelStatusNotice = document.getElementById('modelStatusNotice');
const purgeBtn = document.getElementById('purgeBtn');
const ollamaPill = document.getElementById('ollamaPill');
const ollamaStatusText = document.getElementById('ollamaStatusText');
const fallbackChoiceModal = document.getElementById('fallbackChoiceModal');
const fallbackErrorMsg = document.getElementById('fallbackErrorMsg');
const closeFallbackBtn = document.getElementById('closeFallbackBtn');
const cancelUploadBtn = document.getElementById('cancelUploadBtn');
const processRuleModeBtn = document.getElementById('processRuleModeBtn');
const toast = document.getElementById('toast');

let pendingFilesToUpload = null;
let currentHealth = { ollama_online: false, model_installed: false };

// Initialize
document.addEventListener('DOMContentLoaded', async () => {
  // Ensure spinner is hidden on page load
  if (uploadProgress) {
    uploadProgress.classList.add('hidden');
  }
  await fetchTranslations();
  await checkHealth();
  // Poll Ollama status every 4 seconds
  setInterval(checkHealth, 4000);
  await loadDigest();
  setupEventListeners();
});

async function fetchTranslations() {
  try {
    const res = await fetch('/api/i18n');
    translations = await res.json();
    applyLanguage(currentLang);
  } catch (err) {
    console.error('Failed to load translations:', err);
  }
}

function applyLanguage(lang) {
  currentLang = lang;
  if (!translations[lang]) return;

  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    if (translations[lang][key]) {
      el.textContent = translations[lang][key];
    }
  });

  if (lang === 'en') {
    langEnBtn.classList.add('active');
    langHiBtn.classList.remove('active');
  } else {
    langHiBtn.classList.add('active');
    langEnBtn.classList.remove('active');
  }
}

async function checkHealth() {
  try {
    const selectedModel = modelSelect ? modelSelect.value : 'gemma3:12b';
    const res = await fetch(`/api/health?model=${encodeURIComponent(selectedModel)}`);
    const data = await res.json();
    currentHealth = data;

    // Requirement 1:
    // Green "Gemma 3 12B active" when Ollama and chosen model reachable, red "Rule mode (no model)" otherwise.
    if (data.ollama_online && data.model_installed) {
      ollamaPill.className = 'md3-chip online';
      const label = selectedModel.includes('12b') ? 'Gemma 3 12B active' : (selectedModel.includes('4b') ? 'Gemma 3 4B active' : `${selectedModel} active`);
      ollamaStatusText.textContent = label;
      ollamaPill.setAttribute('title', `Ollama is active with ${data.model}`);
    } else {
      ollamaPill.className = 'md3-chip offline';
      ollamaStatusText.textContent = 'Rule mode (no model)';
      if (!data.ollama_online) {
        ollamaPill.setAttribute('title', "Ollama is offline. Fix: run 'ollama serve' in terminal");
      } else {
        ollamaPill.setAttribute('title', `Model '${selectedModel}' not installed. Fix: run 'ollama pull ${selectedModel}'`);
      }
    }

    updateSettingsModelNotice(data, selectedModel);
  } catch {
    currentHealth = { ollama_online: false, model_installed: false };
    ollamaPill.className = 'md3-chip offline';
    ollamaStatusText.textContent = 'Rule mode (no model)';
    ollamaPill.setAttribute('title', "Ollama is offline. Fix: run 'ollama serve' in terminal");
    updateSettingsModelNotice(currentHealth, modelSelect ? modelSelect.value : 'gemma3:12b');
  }
}

function updateSettingsModelNotice(health, selectedModel) {
  if (!modelStatusNotice) return;

  if (!health.ollama_online) {
    modelStatusNotice.className = '';
    modelStatusNotice.style.background = '#ffebee';
    modelStatusNotice.style.color = '#c62828';
    modelStatusNotice.style.border = '1px solid #ffcdd2';
    modelStatusNotice.innerHTML = `⚠️ <strong>Ollama is offline</strong><br>Start the service: <code>ollama serve</code>`;
  } else if (!health.model_installed) {
    modelStatusNotice.className = '';
    modelStatusNotice.style.background = '#fff3e0';
    modelStatusNotice.style.color = '#e65100';
    modelStatusNotice.style.border = '1px solid #ffe0b2';
    modelStatusNotice.innerHTML = `⚠️ <strong>Model '${escapeHTML(selectedModel)}' is not installed</strong> in Ollama.<br>Run this command to download it:<br><code style="font-size:0.8rem; background:rgba(0,0,0,0.06); padding:2px 6px; border-radius:4px; display:inline-block; margin-top:4px;">ollama pull ${escapeHTML(selectedModel)}</code>`;
  } else {
    modelStatusNotice.className = '';
    modelStatusNotice.style.background = '#e8f5e9';
    modelStatusNotice.style.color = '#1b5e20';
    modelStatusNotice.style.border = '1px solid #a5d6a7';
    modelStatusNotice.innerHTML = `✓ <strong>${escapeHTML(selectedModel)} is installed and active</strong> in Ollama.`;
  }
}

async function loadDigest() {
  try {
    const res = await fetch(`/api/digest?lang=${currentLang}`);
    const digest = await res.json();
    renderDigest(digest);
  } catch (err) {
    digestSummary.textContent = 'Failed to load digest. Please refresh.';
  }
}

function renderDigest(d) {
  digestSummary.textContent = d.summary || 'All clear.';
  if (extractedByMode) {
    extractedByMode.textContent = d.extraction_mode || 'rule mode';
  }

  todayCount.textContent = d.today ? d.today.length : 0;
  tomorrowCount.textContent = d.tomorrow ? d.tomorrow.length : 0;
  weekCount.textContent = d.this_week ? d.this_week.length : 0;
  missedCount.textContent = d.you_may_have_missed ? d.you_may_have_missed.length : 0;
  reviewCount.textContent = d.review_queue ? d.review_queue.length : 0;

  renderItemList(todayList, d.today, 'no_items_today');
  renderItemList(tomorrowList, d.tomorrow, 'no_items_tomorrow');
  renderItemList(weekList, d.this_week, 'no_items_week');
  renderItemList(missedList, d.you_may_have_missed, null);
  renderReviewList(reviewList, d.review_queue);
}

function renderItemList(container, items, emptyKey) {
  container.innerHTML = '';
  if (!items || items.length === 0) {
    if (emptyKey && translations[currentLang]) {
      container.innerHTML = `<p class="md3-empty-state">${translations[currentLang][emptyKey]}</p>`;
    }
    return;
  }

  items.forEach(it => {
    const card = document.createElement('div');
    card.className = 'md3-item-card';
    const dueTime = new Date(it.due_date);
    const timeFormatted = it.has_explicit_time 
      ? dueTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) 
      : 'EOD';

    // Status badge (Postponed, Cancelled, Updated)
    let statusBadge = '';
    if (it.status === 'postponed') {
      statusBadge = '<span class="md3-badge" style="background:#fff3e0; color:#e65100; border:1px solid #ffe0b2;">Postponed</span>';
    } else if (it.status === 'cancelled') {
      statusBadge = '<span class="md3-badge" style="background:#ffebee; color:#c62828; border:1px solid #ffcdd2;">Cancelled</span>';
    } else if (it.status === 'updated') {
      statusBadge = '<span class="md3-badge" style="background:#e3f2fd; color:#1565c0; border:1px solid #bbdefb;">Updated</span>';
    }

    // Extraction mode badge
    let modeBadge = '';
    if (it.extraction_mode === 'gemma') {
      modeBadge = '<span class="md3-badge" style="background:#e8f5e9; color:#1b5e20; border:1px solid #c8e6c9;">Gemma</span>';
    } else {
      modeBadge = '<span class="md3-badge" style="background:#f5f5f5; color:#616161; border:1px solid #e0e0e0;">Rule</span>';
    }

    card.innerHTML = `
      <div class="md3-item-meta">
        <div style="display:flex; align-items:center; gap:0.4rem; flex-wrap:wrap;">
          ${it.subject_or_course ? `<span class="md3-course-pill">${escapeHTML(it.subject_or_course)}</span>` : '<span></span>'}
          ${statusBadge}
          ${modeBadge}
        </div>
        <span class="md3-due-time">🕒 ${timeFormatted}</span>
      </div>
      <h4 class="md3-item-title">${escapeHTML(it.title)}</h4>
      ${it.action_needed ? `<p class="md3-item-action">📌 ${escapeHTML(it.action_needed)}</p>` : ''}
      ${renderEvidenceAccordion(it.evidence)}
    `;
    container.appendChild(card);
  });
}

function renderReviewList(container, items) {
  container.innerHTML = '';
  if (!items || items.length === 0) {
    if (translations[currentLang]) {
      container.innerHTML = `<p class="md3-empty-state">${translations[currentLang]['no_review_items']}</p>`;
    }
    return;
  }

  items.forEach(it => {
    const card = document.createElement('div');
    card.className = 'md3-item-card';
    const dateStr = new Date(it.due_date).toLocaleDateString([], { month: 'short', day: 'numeric' });

    card.innerHTML = `
      <div class="md3-item-meta">
        <span class="md3-course-pill">${escapeHTML(it.subject_or_course || 'Needs Review')}</span>
        <span class="md3-due-time">Due ~ ${dateStr}</span>
      </div>
      <h4 class="md3-item-title">${escapeHTML(it.title)}</h4>
      ${renderEvidenceAccordion(it.evidence)}
      <div class="md3-review-actions">
        <button class="md3-btn md3-btn-primary md3-btn-sm" onclick="confirmItem('${it.id}')">${t('confirm')}</button>
        <button class="md3-btn md3-btn-tonal md3-btn-sm" onclick="dismissItem('${it.id}')">${t('dismiss')}</button>
      </div>
    `;
    container.appendChild(card);
  });
}

function renderEvidenceAccordion(evidence) {
  if (!evidence || evidence.length === 0) return '';
  const count = evidence.length;

  return `
    <details class="md3-evidence-box">
      <summary>💬 WhatsApp Evidence (${count})</summary>
      <div style="margin-top:0.5rem; display:flex; flex-direction:column; gap:0.5rem;">
        ${evidence.map(ev => {
          const dateObj = new Date(ev.timestamp);
          const fullTime = !isNaN(dateObj) ? `${dateObj.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${dateObj.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}` : '';
          return `
          <div class="md3-quote">
            "${escapeHTML(ev.message_text)}"
            <br>
            <span style="font-size:0.75rem; color:var(--md-sys-color-primary);">
              <strong>${escapeHTML(ev.sender || 'Sender')}</strong> &bull; ${fullTime} 
              <span style="text-transform:uppercase; font-size:0.7rem; background:rgba(0,0,0,0.06); padding:2px 5px; border-radius:4px; margin-left:4px;">${escapeHTML(ev.action_taken)}</span>
            </span>
          </div>
        `}).join('')}
      </div>
    </details>
  `;
}

async function confirmItem(id) {
  await fetch(`/api/review/${id}/confirm`, { method: 'POST' });
  showToast('Item confirmed and added to calendar');
  await loadDigest();
}

async function dismissItem(id) {
  await fetch(`/api/review/${id}/dismiss`, { method: 'POST' });
  showToast('Item dismissed');
  await loadDigest();
}

function setupEventListeners() {
  browseBtn.addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', handleFileSelect);

  dropzone.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropzone.classList.add('dragover');
  });
  dropzone.addEventListener('dragleave', () => dropzone.classList.remove('dragover'));
  dropzone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropzone.classList.remove('dragover');
    if (e.dataTransfer.files.length) {
      uploadFiles(e.dataTransfer.files);
    }
  });

  langEnBtn.addEventListener('click', () => { applyLanguage('en'); loadDigest(); });
  langHiBtn.addEventListener('click', () => { applyLanguage('hi'); loadDigest(); });

  copyDigestBtn.addEventListener('click', async () => {
    const res = await fetch(`/api/digest/plain?lang=${currentLang}`);
    const text = await res.text();
    await navigator.clipboard.writeText(text);
    showToast(t('copied'));
  });

  settingsBtn.addEventListener('click', () => {
    settingsModal.classList.remove('hidden');
    checkHealth();
  });
  closeSettingsBtn.addEventListener('click', () => settingsModal.classList.add('hidden'));

  if (modelSelect) {
    modelSelect.addEventListener('change', () => {
      checkHealth();
    });
  }

  // Fallback modal buttons
  if (closeFallbackBtn) {
    closeFallbackBtn.addEventListener('click', () => {
      fallbackChoiceModal.classList.add('hidden');
      pendingFilesToUpload = null;
    });
  }
  if (cancelUploadBtn) {
    cancelUploadBtn.addEventListener('click', () => {
      fallbackChoiceModal.classList.add('hidden');
      pendingFilesToUpload = null;
      showToast('Processing cancelled');
    });
  }
  if (processRuleModeBtn) {
    processRuleModeBtn.addEventListener('click', async () => {
      fallbackChoiceModal.classList.add('hidden');
      if (pendingFilesToUpload) {
        const files = pendingFilesToUpload;
        pendingFilesToUpload = null;
        await executeUpload(files, 'rule');
      }
    });
  }

  purgeBtn.addEventListener('click', async () => {
    if (confirm(t('purge_confirm'))) {
      await fetch('/api/purge', { method: 'POST' });
      showToast('All local data purged');
      settingsModal.classList.add('hidden');
      await loadDigest();
    }
  });
}

function handleFileSelect(e) {
  if (e.target.files.length) {
    uploadFiles(e.target.files);
  }
}

async function uploadFiles(files) {
  // If Ollama is offline or model is not installed according to health check,
  // immediately show fallback modal so we NEVER silently fall back!
  if (!currentHealth.ollama_online || !currentHealth.model_installed) {
    pendingFilesToUpload = files;
    const reason = !currentHealth.ollama_online 
      ? "Ollama service is unreachable at http://127.0.0.1:11434 ('ollama serve')."
      : `Selected model '${escapeHTML(modelSelect.value)}' is not installed in Ollama.`;
    fallbackErrorMsg.innerHTML = `${reason}<br><br><strong>Aalsi never silently falls back.</strong> Please choose an action:`;
    fallbackChoiceModal.classList.remove('hidden');
    return;
  }

  await executeUpload(files, 'auto');
}

async function executeUpload(files, mode) {
  uploadProgress.classList.remove('hidden');
  const formData = new FormData();
  for (let i = 0; i < files.length; i++) {
    formData.append('files', files[i]);
  }
  formData.append('mode', mode || 'auto');

  try {
    const res = await fetch('/api/upload', {
      method: 'POST',
      body: formData,
    });
    
    if (res.status === 503) {
      // Ollama unreachable during processing!
      const errData = await res.json();
      pendingFilesToUpload = files;
      fallbackErrorMsg.textContent = errData.message || "Ollama is unreachable.";
      fallbackChoiceModal.classList.remove('hidden');
      return;
    }

    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }

    const result = await res.json();
    showToast(`Imported ${result.imported_messages} messages! [Mode: ${mode === 'rule' ? 'Rule mode' : 'Gemma'}]`);
    await loadDigest();
  } catch (err) {
    showToast('Failed to parse chat export');
  } finally {
    uploadProgress.classList.add('hidden');
  }
}

function t(key) {
  if (translations[currentLang] && translations[currentLang][key]) {
    return translations[currentLang][key];
  }
  return key;
}

function showToast(msg) {
  toast.textContent = msg;
  toast.classList.remove('hidden');
  setTimeout(() => toast.classList.add('hidden'), 3000);
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag));
}
