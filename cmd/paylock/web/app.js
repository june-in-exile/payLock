// Wallet module lazy loader
let walletModule = null;
let walletLoading = null;

async function loadWallet() {
  if (walletModule) return walletModule;
  if (walletLoading) return walletLoading;
  walletLoading = import('./wallet.js').then(function(mod) {
    walletModule = mod;
    mod.onWalletConnect(recheckAccessAfterConnect);
    mod.initWallet();
    return mod;
  }).catch(function(err) {
    walletLoading = null;
    throw err;
  });
  return walletLoading;
}

function isWalletConnected() {
  return walletModule ? walletModule.isWalletConnected() : false;
}

function getWalletAddress() {
  return walletModule ? walletModule.getWalletAddress() : null;
}

// State
let currentView = 'videos';
let pollInterval = null;
let navGeneration = 0;

// Navigation
function navigate(view, params, push) {
  if (push === undefined) push = true;
  navGeneration++;
  currentView = view;

  if (pollInterval) {
    if (typeof pollInterval.close === 'function') {
      pollInterval.close();
    } else {
      clearInterval(pollInterval);
    }
    pollInterval = null;
  }

  document.querySelectorAll('.view').forEach(function(v) { v.classList.remove('active'); });
  const viewEl = document.getElementById('view-' + view);
  if (viewEl) viewEl.classList.add('active');

  if (view === 'videos') {
    loadVideos();
    if (push) history.pushState({view: view}, '', '/');
  } else if (view === 'player' && params && params.id) {
    if (push) history.pushState({view: view, id: params.id}, '', '/play/' + params.id);
    loadPlayer(params.id);
  }
}

// Video list
function createVideoCard(v) {
  const card = document.createElement('div');
  card.className = 'video-card';

  const thumb = document.createElement('div');
  thumb.className = 'video-thumb';
  thumb.addEventListener('click', function() { navigate('player', { id: v.id }); });
  thumb.style.cursor = 'pointer';

  if (v.thumbnail_blob_url && v.status === 'ready') {
    const img = document.createElement('img');
    img.src = v.thumbnail_blob_url;
    img.alt = v.title || v.id;
    img.style.cssText = 'width:100%;height:100%;object-fit:cover;';
    thumb.appendChild(img);
    const playOverlay = document.createElement('div');
    playOverlay.className = 'play-overlay';
    playOverlay.textContent = '\u25B6';
    thumb.appendChild(playOverlay);
  } else {
    thumb.textContent = v.status === 'ready' ? '\u25B6' : '\u2026';
  }

  card.appendChild(thumb);

  const info = document.createElement('div');
  info.className = 'video-info';

  const titleEl = document.createElement('div');
  titleEl.className = 'video-id';
  titleEl.textContent = v.title || v.id;
  titleEl.style.cursor = 'pointer';
  titleEl.addEventListener('click', function() { navigate('player', { id: v.id }); });
  info.appendChild(titleEl);

  const meta = document.createElement('div');
  meta.className = 'video-meta';

  const idEl = document.createElement('span');
  idEl.textContent = 'ID: ' + v.id.slice(0, 8);
  idEl.style.fontFamily = 'monospace';
  meta.appendChild(idEl);

  const statusBadge = document.createElement('span');
  const safeStatus = ['ready', 'processing', 'failed'].includes(v.status) ? v.status : 'failed';
  statusBadge.className = 'status-badge ' + safeStatus;
  statusBadge.textContent = v.status;
  meta.appendChild(statusBadge);

  const priceBadge = document.createElement('span');
  if (v.price > 0) {
    priceBadge.className = 'price-badge paid';
    priceBadge.textContent = (v.price / 1_000_000_000).toFixed(2) + ' SUI';
  } else {
    priceBadge.className = 'price-badge free';
    priceBadge.textContent = 'Free';
  }
  meta.appendChild(priceBadge);

  const dateEl = document.createElement('span');
  dateEl.textContent = formatDate(v.created_at);
  meta.appendChild(dateEl);

  info.appendChild(meta);
  card.appendChild(info);

  const actions = document.createElement('div');
  actions.className = 'video-actions';

  const deleteBtn = document.createElement('button');
  deleteBtn.className = 'btn btn-sm btn-danger';
  deleteBtn.textContent = 'Delete';
  deleteBtn.addEventListener('click', function(e) {
    e.stopPropagation();
    deleteVideo(v.id);
  });
  actions.appendChild(deleteBtn);

  card.appendChild(actions);

  return card;
}

function setEmptyState(container, message) {
  container.textContent = '';
  const wrapper = document.createElement('div');
  wrapper.className = 'empty-state';
  const p = document.createElement('p');
  p.textContent = message;
  wrapper.appendChild(p);
  container.appendChild(wrapper);
}

async function deleteVideo(id) {
  if (!confirm('Are you sure you want to delete this video? This action cannot be undone.')) {
    return;
  }
  try {
    const res = await fetch('/api/videos/' + encodeURIComponent(id), {
      method: 'DELETE',
    });
    if (!res.ok) {
      const data = await res.json().catch(function() { return {}; });
      alert(data.error || 'Failed to delete video.');
      return;
    }
    navigate('videos');
  } catch (err) {
    alert('Failed to delete video: ' + err.message);
  }
}

async function loadVideos() {
  const container = document.getElementById('video-list');
  try {
    const res = await fetch('/api/videos');
    if (!res.ok) {
      setEmptyState(container, 'Failed to load videos.');
      return;
    }
    const data = await res.json();
    const videos = data.videos || [];

    if (videos.length === 0) {
      setEmptyState(container, 'No videos yet. Drag and drop a file or click Select MP4.');
      return;
    }

    container.textContent = '';
    const grid = document.createElement('div');
    grid.className = 'video-grid';
    videos.forEach(function(v) { grid.appendChild(createVideoCard(v)); });
    container.appendChild(grid);
  } catch (err) {
    setEmptyState(container, 'Cannot connect to server.');
  }
}

// Upload
const uploadOverlay = document.getElementById('upload-overlay');
const uploadFile = document.getElementById('upload-file');
const uploadProgress = document.getElementById('upload-progress');
const progressFill = document.getElementById('progress-fill');
const progressText = document.getElementById('progress-text');
const uploadResult = document.getElementById('upload-result');

window.addEventListener('dragover', function(e) {
  e.preventDefault();
  uploadOverlay.classList.add('active');
});

uploadOverlay.addEventListener('dragleave', function(e) {
  e.preventDefault();
  uploadOverlay.classList.remove('active');
});

window.addEventListener('drop', function(e) {
  e.preventDefault();
  uploadOverlay.classList.remove('active');
  if (e.dataTransfer.files.length > 0) {
    stageFile(e.dataTransfer.files[0]);
  }
});

uploadFile.addEventListener('change', function() {
  if (uploadFile.files.length > 0) {
    stageFile(uploadFile.files[0]);
  }
});

// Staged file waiting for user confirmation
let stagedFile = null;
const stagedFileEl = document.getElementById('staged-file');
const stagedPreview = document.getElementById('staged-preview');
const stagedFileName = document.getElementById('staged-file-name');
const stagedFileSize = document.getElementById('staged-file-size');

function formatFileSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

function stageFile(file) {
  if (!file.name.toLowerCase().endsWith('.mp4')) {
    showResult('error', 'Only MP4 files are accepted.');
    return;
  }

  if (stagedPreview.src) {
    URL.revokeObjectURL(stagedPreview.src);
  }

  stagedFile = file;
  stagedFileName.textContent = file.name;
  stagedFileSize.textContent = formatFileSize(file.size);
  stagedPreview.src = URL.createObjectURL(file);
  stagedFileEl.classList.add('active');
  uploadResult.style.display = 'none';
  updateUploadButton();
}

const uploadActionBtn = document.getElementById('upload-action-btn');

function updateUploadButton() {
  if (stagedFile) {
    uploadActionBtn.textContent = 'Confirm Upload';
  } else {
    uploadActionBtn.textContent = 'Select MP4';
  }
}

window.handleUploadAction = function() {
  if (stagedFile) {
    confirmUpload();
  } else {
    document.getElementById('upload-file').click();
  }
};

window.cancelStaged = function() {
  stagedFile = null;
  stagedFileEl.classList.remove('active');
  if (stagedPreview.src) {
    URL.revokeObjectURL(stagedPreview.src);
    stagedPreview.removeAttribute('src');
  }
  uploadFile.value = '';
  updateUploadButton();
};

function sendUpload(formData, file) {
  return new Promise(function(resolve, reject) {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/upload');

    xhr.upload.addEventListener('progress', function(e) {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        progressFill.style.width = pct + '%';
        progressText.textContent = 'Uploading ' + file.name + '... ' + pct + '%';
      }
    });

    xhr.addEventListener('load', function() {
      if (xhr.status === 202) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        let msg = 'Upload failed.';
        try { msg = JSON.parse(xhr.responseText).error || msg; } catch(e) {}
        reject(new Error(msg));
      }
    });

    xhr.addEventListener('error', function() {
      reject(new Error('Network error. Is the server running?'));
    });

    xhr.send(formData);
  });
}

function pollUntilReady(id) {
  return new Promise(function(resolve, reject) {
    if (typeof EventSource !== 'undefined') {
      var es = new EventSource('/api/status/' + encodeURIComponent(id) + '/events');
      es.onmessage = function(e) {
        var video = JSON.parse(e.data);
        if (video.status === 'ready') {
          es.close();
          resolve(video);
        } else if (video.status === 'failed') {
          es.close();
          reject(new Error(video.error || 'Upload failed'));
        }
      };
      es.onerror = function() {
        es.close();
        pollFallback(id).then(resolve, reject);
      };
      return;
    }
    pollFallback(id).then(resolve, reject);
  });
}

function pollFallback(id) {
  return new Promise(function(resolve, reject) {
    var interval = setInterval(async function() {
      try {
        var res = await fetch('/api/status/' + encodeURIComponent(id));
        if (!res.ok) return;
        var video = await res.json();
        if (video.status === 'ready') {
          clearInterval(interval);
          resolve(video);
        } else if (video.status === 'failed') {
          clearInterval(interval);
          reject(new Error(video.error || 'Upload failed'));
        }
      } catch (err) {
        clearInterval(interval);
        reject(err);
      }
    }, 1000);
  });
}

async function confirmUpload() {
  if (!stagedFile) return;
  const file = stagedFile;

  stagedFileEl.classList.remove('active');
  uploadResult.className = 'upload-result';
  uploadResult.style.display = 'none';
  uploadProgress.style.display = '';
  progressFill.style.width = '0%';
  progressText.textContent = 'Uploading ' + file.name + '...';

  const formData = new FormData();
  formData.append('video', file);
  const title = document.getElementById('video-title').value;
  if (title) {
    formData.append('title', title);
  }
  const priceInput = document.getElementById('video-price').value;
  let priceMist = 0;
  if (priceInput && parseFloat(priceInput) > 0) {
    priceMist = Math.round(parseFloat(priceInput) * 1_000_000_000);
    formData.append('price', priceMist.toString());
  }
  const walletAddr = getWalletAddress();
  if (walletAddr) {
    formData.append('creator', walletAddr);
  }

  const fileArrayBuffer = priceMist > 0 ? await file.arrayBuffer() : null;

  stagedFile = null;
  if (stagedPreview.src) {
    URL.revokeObjectURL(stagedPreview.src);
    stagedPreview.removeAttribute('src');
  }
  updateUploadButton();

  try {
    const data = await sendUpload(formData, file);
    uploadFile.value = '';
    document.getElementById('video-title').value = '';
    document.getElementById('video-price').value = '';

    if (priceMist > 0 && fileArrayBuffer) {
      if (!isWalletConnected()) {
        throw new Error('Wallet must be connected to encrypt paid videos');
      }

      // Encrypt + upload full blob runs in parallel with backend's preview upload
      progressText.textContent = 'Encrypting & uploading to Walrus + Sui...';
      const mod = await loadWallet();
      const [video, encResult] = await Promise.all([
        pollUntilReady(data.id),
        mod.encryptAndPublish(data.id, fileArrayBuffer, priceMist),
      ]);

      // Both done — update preview + full blob IDs on-chain in a single tx
      progressText.textContent = 'Publishing blob IDs on-chain...';
      await mod.updateBlobIds(encResult.suiObjectId, data.id, video.preview_blob_id, encResult.fullBlobId);
    }

    uploadProgress.style.display = 'none';
    navigate('player', { id: data.id });
  } catch (err) {
    uploadProgress.style.display = 'none';
    showResult('error', err.message);
  }
}

function showResult(type, message) {
  uploadResult.className = 'upload-result ' + type;
  uploadResult.textContent = message;
  uploadResult.style.display = 'block';
}

// Player
let currentVideo = null;

async function recheckAccessAfterConnect() {
  if (!currentVideo || currentView !== 'player') return;
  if (currentVideo.price <= 0 || !currentVideo.encrypted || !currentVideo.sui_object_id) return;
  if (!isWalletConnected()) return;

  const videoEl = document.getElementById('video-player');
  if (videoEl && videoEl.src && videoEl.src.startsWith('blob:')) return;

  const urlEl = document.getElementById('stream-url');
  const hintEl = document.getElementById('paywall-hint');

  try {
    const mod = await loadWallet();
    const passId = await mod.findAccessPass(currentVideo.sui_object_id);
    if (!passId) return;
    if (urlEl) urlEl.textContent = 'Decrypting full video...';
    var paywallEl = document.getElementById('paywall-overlay');
    if (paywallEl) paywallEl.style.display = 'none';
    await mod.decryptAndPlay(currentVideo);
    updateChainStatus(currentVideo);
  } catch (err) {
    if (hintEl) hintEl.textContent = 'Decryption failed: ' + err.message;
  }
}

function loadPlayer(id) {
  const videoEl = document.getElementById('video-player');
  const videoIdEl = document.getElementById('player-video-id');
  const paywallEl = document.getElementById('paywall-overlay');

  videoIdEl.textContent = id;
  videoEl.removeAttribute('src');
  paywallEl.style.display = 'none';
  document.getElementById('player-loading').style.display = 'none';
  currentVideo = null;

  checkAndPlay(id, navGeneration);
}

function setupPreviewEndHandler(videoEl, video) {
  videoEl.onended = null;
  if (video.price > 0) {
    videoEl.onended = function() {
      const paywallEl = document.getElementById('paywall-overlay');
      const priceEl = document.getElementById('paywall-price');
      const hintEl = document.getElementById('paywall-hint');
      priceEl.textContent = (video.price / 1_000_000_000).toFixed(2) + ' SUI';
      const btn = document.getElementById('paywall-unlock-btn');
      btn.disabled = false;
      btn.textContent = 'Purchase & Unlock';
      if (!isWalletConnected()) {
        hintEl.textContent = 'Connect wallet to purchase';
      } else if (!video.sui_object_id) {
        hintEl.textContent = 'Video not yet published on-chain';
      } else {
        hintEl.textContent = '';
      }
      paywallEl.style.display = 'flex';
    };
  }
}

function playPreview(videoEl, urlEl, video) {
  urlEl.textContent = video.preview_blob_url;
  videoEl.src = video.preview_blob_url;
  videoEl.play().catch(function() {});
}

async function startPlayback(video) {
  const videoEl = document.getElementById('video-player');
  const urlEl = document.getElementById('stream-url');

  currentVideo = video;

  // Free video: play full directly
  if (video.price === 0 && video.full_blob_url) {
    urlEl.textContent = video.full_blob_url;
    videoEl.src = video.full_blob_url;
    videoEl.play().catch(function() {});
    updateChainStatus(video);
    return;
  }

  // Paid video with AccessPass: auto-decrypt
  if (video.price > 0 && video.encrypted && video.sui_object_id && isWalletConnected()) {
    try {
      const mod = await loadWallet();
      if (await mod.findAccessPass(video.sui_object_id)) {
        urlEl.textContent = 'Decrypting full video...';
        await mod.decryptAndPlay(video);
        updateChainStatus(video);
        return;
      }
    } catch (err) {
      document.getElementById('paywall-hint').textContent = 'Auto-decrypt failed: ' + err.message;
    }
  }

  // Default: play preview, show paywall on end
  playPreview(videoEl, urlEl, video);
  setupPreviewEndHandler(videoEl, video);
  updateChainStatus(video);
}

function updateChainStatus(video) {
  const chainEl = document.getElementById('chain-status');
  if (!chainEl) return;

  if (video.sui_object_id) {
    const url = 'https://suiscan.xyz/testnet/object/' + video.sui_object_id + '/tx-blocks';
    let statusHtml = '<span style="color:var(--success);">On-chain</span> ' +
      '<a href="' + url + '" target="_blank" rel="noopener noreferrer" style="font-family:monospace; color:var(--accent); font-size:0.75rem; text-decoration:none; border-bottom:1px dashed var(--accent);">' +
      video.sui_object_id.slice(0, 10) + '...' + video.sui_object_id.slice(-4) + '</a>';
    if (video.encrypted) {
      statusHtml += ' <span style="color:var(--accent); font-size:0.75rem;">Seal encrypted</span>';
    }
    chainEl.style.display = 'block';
    chainEl.innerHTML = statusHtml;
    return;
  }

  if (video.price > 0 && !video.full_blob_id) {
    chainEl.style.display = 'block';
    chainEl.innerHTML = '<span style="color:var(--warning);">Awaiting Seal encryption & on-chain publish</span>';
  } else if (video.price > 0) {
    chainEl.style.display = 'block';
    chainEl.innerHTML = '<span style="color:var(--text-muted);">Connect wallet to publish on-chain</span>';
  } else {
    chainEl.style.display = 'none';
  }
}

async function purchaseAndPlay() {
  if (!currentVideo) return;
  const btn = document.getElementById('paywall-unlock-btn');
  const hintEl = document.getElementById('paywall-hint');

  if (!isWalletConnected()) {
    loadWallet().then(function(mod) { mod.connectWallet(); });
    return;
  }

  if (!currentVideo.sui_object_id) {
    if (hintEl) hintEl.textContent = 'Video not published on-chain yet';
    return;
  }

  btn.disabled = true;
  if (hintEl) hintEl.textContent = '';

  try {
    const mod = await loadWallet();

    btn.textContent = 'Checking access...';
    let accessPassId = await mod.findAccessPass(currentVideo.sui_object_id);

    if (!accessPassId) {
      btn.textContent = 'Purchasing...';
      accessPassId = await mod.purchaseVideo(currentVideo);
    }
    console.log("accessPassId: ", accessPassId);

    btn.textContent = 'Decrypting...';
    await mod.decryptAndPlay(currentVideo, accessPassId);
  } catch (err) {
    btn.disabled = false;
    btn.textContent = 'Purchase & Unlock';
    if (hintEl) hintEl.textContent = 'Failed: ' + err.message;
  }
}

async function checkAndPlay(id, generation) {
  const statusEl = document.getElementById('player-status');
  const urlEl = document.getElementById('stream-url');
  const titleEl = document.getElementById('player-video-title');

  if (generation !== navGeneration) return;

  try {
    const res = await fetch('/api/status/' + encodeURIComponent(id));

    if (generation !== navGeneration) return;

    if (!res.ok) {
      statusEl.textContent = 'not found';
      statusEl.className = 'status-badge failed';
      return;
    }
    const video = await res.json();
    titleEl.textContent = video.title || video.id;
    const safeStatus = ['ready', 'processing', 'failed'].includes(video.status) ? video.status : 'failed';
    statusEl.textContent = video.status;
    statusEl.className = 'status-badge ' + safeStatus;

    if (video.status === 'ready') {
      startPlayback(video);
    } else if (video.status === 'processing') {
      var loadingEl = document.getElementById('player-loading');
      loadingEl.style.display = 'flex';
      urlEl.textContent = '';

      function onVideoReady(v) {
        if (generation !== navGeneration) return;
        loadingEl.style.display = 'none';
        titleEl.textContent = v.title || v.id;
        statusEl.textContent = v.status;
        statusEl.className = 'status-badge ready';
        startPlayback(v);
      }
      function onVideoFailed(v) {
        if (generation !== navGeneration) return;
        loadingEl.style.display = 'none';
        statusEl.textContent = 'failed';
        statusEl.className = 'status-badge failed';
        urlEl.textContent = 'Upload failed: ' + (v.error || 'unknown error');
      }

      if (typeof EventSource !== 'undefined') {
        var es = new EventSource('/api/status/' + encodeURIComponent(id) + '/events');
        pollInterval = es;
        es.onmessage = function(e) {
          if (generation !== navGeneration) { es.close(); return; }
          var v = JSON.parse(e.data);
          if (v.status === 'ready') { es.close(); pollInterval = null; onVideoReady(v); }
          else if (v.status === 'failed') { es.close(); pollInterval = null; onVideoFailed(v); }
        };
        es.onerror = function() {
          es.close();
          startPollingFallback();
        };
      } else {
        startPollingFallback();
      }

      function startPollingFallback() {
        pollInterval = setInterval(async function() {
          if (generation !== navGeneration) { clearInterval(pollInterval); pollInterval = null; return; }
          try {
            var r = await fetch('/api/status/' + encodeURIComponent(id));
            if (generation !== navGeneration) { clearInterval(pollInterval); pollInterval = null; return; }
            if (!r.ok) return;
            var v = await r.json();
            if (v.status === 'ready') { clearInterval(pollInterval); pollInterval = null; onVideoReady(v); }
            else if (v.status === 'failed') { clearInterval(pollInterval); pollInterval = null; onVideoFailed(v); }
          } catch (err) { clearInterval(pollInterval); pollInterval = null; }
        }, 1000);
      }
    } else {
      urlEl.textContent = 'Upload failed: ' + (video.error || 'unknown error');
    }
  } catch (err) {
    if (generation !== navGeneration) return;
    statusEl.textContent = 'error';
    statusEl.className = 'status-badge failed';
    urlEl.textContent = 'Cannot connect to server.';
  }
}

function copyStreamUrl() {
  const url = document.getElementById('stream-url').textContent;
  navigator.clipboard.writeText(url).then(function() {
    const btn = document.querySelector('.player-actions .btn-outline');
    btn.textContent = 'Copied!';
    setTimeout(function() { btn.textContent = 'Copy URL'; }, 1500);
  });
}

function deleteFromPlayer() {
  const id = document.getElementById('player-video-id').textContent;
  if (!id) return;
  deleteVideo(id);
}

// Utilities
function formatDate(isoStr) {
  if (!isoStr) return '';
  const d = new Date(isoStr);
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

// Expose globals for HTML onclick handlers
window.navigate = navigate;
window.purchaseAndPlay = purchaseAndPlay;
window.copyStreamUrl = copyStreamUrl;
window.deleteFromPlayer = deleteFromPlayer;

window.walletConnect = async function() {
  const mod = await loadWallet();
  mod.connectWallet();
};

// Router
function handleRoute() {
  const path = window.location.pathname;
  if (path === '/') {
    navigate('videos', null, false);
  } else if (path.startsWith('/play/')) {
    const id = path.slice(6);
    navigate('player', { id: id }, false);
  } else {
    navigate('videos', null, false);
  }
}

window.addEventListener('popstate', handleRoute);
handleRoute();

// Eagerly start wallet loading (non-blocking) for auto-reconnect
loadWallet().catch(function() {});
