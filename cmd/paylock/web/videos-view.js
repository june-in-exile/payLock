import { html, useState, useEffect, useRef, useCallback } from './lib.js';
import {
  stagedFile, uploadState, navigate, showToast,
  formatDate, formatFileSize, formatSui,
  stageNewFile, clearStagedFile, loadWallet, walletState,
  navGeneration,
} from './state.js';

function stepCls(status) {
  return 'upload-step ' + (status || 'pending');
}

function UploadSteps({ step, previewDone, browserStep }) {
  const isOnchain = step === 'onchain';
  const allDone = step === 'allDone';

  const previewStatus = (previewDone || isOnchain || allDone) ? 'done' : 'active';

  let encryptStatus, walrusStatus;
  if (isOnchain || allDone) {
    encryptStatus = 'done';
    walrusStatus = 'done';
  } else if (browserStep === 'encrypt') {
    encryptStatus = 'active';
    walrusStatus = 'pending';
  } else if (browserStep === 'walrus') {
    encryptStatus = 'done';
    walrusStatus = 'active';
  } else {
    encryptStatus = 'done';
    walrusStatus = 'done';
  }

  const onchainStatus = allDone ? 'done' : isOnchain ? 'active' : 'pending';

  return html`
    <div class="upload-steps">
      <div class="upload-steps-parallel">
        <span class="upload-parallel-badge">parallel</span>
        <div class="upload-track">
          <div class="upload-track-label">Server</div>
          <div class=${stepCls(previewStatus)}>
            <span class="upload-step-icon"></span>
            <span>Uploading preview</span>
          </div>
        </div>
        <div class="upload-track">
          <div class="upload-track-label">Browser</div>
          <div class=${stepCls(encryptStatus)}>
            <span class="upload-step-icon"></span>
            <span>Encrypting with Seal</span>
          </div>
          <div class=${stepCls(walrusStatus)}>
            <span class="upload-step-icon"></span>
            <span>Uploading encrypted blob</span>
          </div>
        </div>
      </div>
      <div class=${stepCls(onchainStatus)}>
        <span class="upload-step-icon"></span>
        <span>Creating video on-chain</span>
      </div>
    </div>
  `;
}

function UploadProgress() {
  const state = uploadState.value;
  if (!state.active) return null;

  return html`
    <div class="upload-progress">
      ${state.showSpinner
        ? html`<div class="upload-spinner"><div class="upload-spinner-ring"></div></div>`
        : html`<div class="progress-bar"><div class="fill" style="width: ${state.percent}%"></div></div>`}
      <div class="progress-text">${state.text}</div>
      ${state.showSteps && html`<${UploadSteps} step=${state.step} previewDone=${state.previewDone} browserStep=${state.browserStep} />`}
    </div>
  `;
}

function StagedFilePreview() {
  const staged = stagedFile.value;
  const videoRef = useRef(null);

  useEffect(() => {
    if (videoRef.current && staged) {
      videoRef.current.src = staged.objectUrl;
    }
  }, [staged]);

  if (!staged) return null;

  return html`
    <div class="staged-file active">
      <div class="staged-file-preview">
        <video ref=${videoRef} muted></video>
      </div>
      <div class="staged-file-info">
        <div class="staged-file-name">${staged.name}</div>
        <div class="staged-file-size">${formatFileSize(staged.size)}</div>
        <div class="staged-file-actions">
          <button class="btn btn-outline" onclick=${() => fileInputRef.current && fileInputRef.current.click()}>
            Change File
          </button>
          <button class="btn btn-outline" onclick=${clearStagedFile}>Remove</button>
        </div>
      </div>
    </div>
  `;
}

// Shared ref for the hidden file input (set by VideosView)
let fileInputRef = { current: null };

function VideoCard({ video }) {
  const safeStatus = ['ready', 'processing', 'failed'].includes(video.status) ? video.status : 'failed';
  const isPaid = video.price > 0;

  return html`
    <div class="video-card">
      <div class="video-thumb" style="cursor:pointer" onclick=${() => navigate('player', { id: video.id })}>
        ${video.thumbnail_blob_url && video.status === 'ready'
          ? html`
              <img src=${video.thumbnail_blob_url} alt=${video.title || video.id}
                style="width:100%;height:100%;object-fit:cover;" />
              <div class="play-overlay">\u25B6</div>
            `
          : video.status === 'ready' ? '\u25B6' : '\u2026'}
      </div>
      <div class="video-info">
        <div class="video-id" style="cursor:pointer" onclick=${() => navigate('player', { id: video.id })}>
          ${video.title || video.id}
        </div>
        <div class="video-meta">
          <span style="font-family:monospace">ID: ${video.id.slice(0, 8)}</span>
          <span class=${'status-badge ' + safeStatus}>${video.status}</span>
          <span class=${isPaid ? 'price-badge paid' : 'price-badge free'}>
            ${isPaid ? formatSui(video.price) + ' SUI' : 'Free'}
          </span>
          <span>${formatDate(video.created_at)}</span>
        </div>
      </div>
      <div class="video-actions">
        <button class="btn btn-sm btn-danger" onclick=${(e) => { e.stopPropagation(); deleteVideo(video.id); }}>
          Delete
        </button>
      </div>
    </div>
  `;
}

async function deleteVideo(id) {
  if (!confirm('Are you sure you want to delete this video? This action cannot be undone.')) return;
  try {
    const res = await fetch('/api/videos/' + encodeURIComponent(id), { method: 'DELETE' });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      alert(data.error || 'Failed to delete video.');
      return;
    }
    navigate('videos');
  } catch (err) {
    alert('Failed to delete video: ' + err.message);
  }
}

function sendUpload(formData, fileName) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/upload');

    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        uploadState.value = { ...uploadState.value, percent: pct, text: 'Uploading ' + fileName + '... ' + pct + '%' };
      }
    });

    xhr.addEventListener('load', () => {
      if (xhr.status === 202) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        let msg = 'Upload failed.';
        try { msg = JSON.parse(xhr.responseText).error || msg; } catch (_) { /* parse error is ok */ }
        reject(new Error(msg));
      }
    });

    xhr.addEventListener('error', () => reject(new Error('Network error. Is the server running?')));
    xhr.send(formData);
  });
}

function pollUntilReady(id) {
  return new Promise((resolve, reject) => {
    if (typeof EventSource !== 'undefined') {
      const es = new EventSource('/api/status/' + encodeURIComponent(id) + '/events');
      es.onmessage = (e) => {
        const video = JSON.parse(e.data);
        if (video.status === 'ready') { es.close(); resolve(video); }
        else if (video.status === 'failed') { es.close(); reject(new Error(video.error || 'Upload failed')); }
      };
      es.onerror = () => { es.close(); pollFallback(id).then(resolve, reject); };
      return;
    }
    pollFallback(id).then(resolve, reject);
  });
}

function pollFallback(id) {
  return new Promise((resolve, reject) => {
    const interval = setInterval(async () => {
      try {
        const res = await fetch('/api/status/' + encodeURIComponent(id));
        if (!res.ok) return;
        const video = await res.json();
        if (video.status === 'ready') { clearInterval(interval); resolve(video); }
        else if (video.status === 'failed') { clearInterval(interval); reject(new Error(video.error || 'Upload failed')); }
      } catch (err) { clearInterval(interval); reject(err); }
    }, 1000);
  });
}

async function confirmUpload(fileInput) {
  const staged = stagedFile.value;
  if (!staged) return;

  const priceInput = document.getElementById('video-price').value;
  let priceMist = 0;
  if (priceInput && parseFloat(priceInput) > 0) {
    priceMist = Math.round(parseFloat(priceInput) * 1_000_000_000);
  }
  if (priceMist > 0 && !walletState.value.connected) {
    showToast('error', 'Please connect your wallet before uploading a paid video.', 5000);
    return;
  }

  const file = staged.file;
  const fileArrayBuffer = priceMist > 0 ? await file.arrayBuffer() : null;

  clearStagedFile();
  uploadState.value = { active: true, percent: 0, text: 'Uploading ' + file.name + '...', step: null, showSpinner: false, showSteps: false };

  const formData = new FormData();
  formData.append('video', file);
  const title = document.getElementById('video-title').value;
  if (title) formData.append('title', title);
  if (priceMist > 0) formData.append('price', priceMist.toString());
  const walletAddr = walletState.value.address;
  if (walletAddr) formData.append('creator', walletAddr);

  try {
    const data = await sendUpload(formData, file.name);
    if (fileInput) fileInput.value = '';
    document.getElementById('video-title').value = '';
    document.getElementById('video-price').value = '';

    if (priceMist > 0 && fileArrayBuffer) {
      uploadState.value = { ...uploadState.value, showSpinner: true, showSteps: true, step: 'parallel', previewDone: false, browserStep: 'encrypt', text: 'Encrypting & uploading...' };

      const mod = await loadWallet();

      const [video, encResult] = await Promise.all([
        pollUntilReady(data.id).then((v) => {
          uploadState.value = { ...uploadState.value, previewDone: true };
          return v;
        }),
        mod.encryptAndPublish(fileArrayBuffer, (browserStep) => {
          uploadState.value = { ...uploadState.value, browserStep };
          if (browserStep === 'walrus') uploadState.value = { ...uploadState.value, text: 'Uploading encrypted blob to Walrus...' };
        }),
      ]);

      uploadState.value = { ...uploadState.value, step: 'onchain', previewDone: true, browserStep: 'done', text: 'Creating video on-chain...' };
      await mod.createVideoOnChain(data.id, priceMist, video.preview_blob_id, encResult.fullBlobId, encResult.namespace);
    }

    uploadState.value = { active: false, percent: 0, text: '', step: null, showSpinner: false, showSteps: false };
    showToast('success', 'Upload complete!');
    navigate('player', { id: data.id });
  } catch (err) {
    uploadState.value = { active: false, percent: 0, text: '', step: null, showSpinner: false, showSteps: false };
    showToast('error', 'Upload failed: ' + err.message, 5000);
  }
}

export function VideosView() {
  const [videos, setVideos] = useState([]);
  const [loadError, setLoadError] = useState(null);
  const inputRef = useRef(null);
  fileInputRef = inputRef;
  const staged = stagedFile.value;

  const generation = navGeneration.value;
  useEffect(() => {
    let cancelled = false;
    fetch('/api/videos')
      .then((res) => {
        if (!res.ok) throw new Error('Failed to load');
        return res.json();
      })
      .then((data) => { if (!cancelled) setVideos(data.videos || []); })
      .catch(() => { if (!cancelled) setLoadError('Cannot connect to server.'); });
    return () => { cancelled = true; };
  }, [generation]);

  const handleUploadAction = useCallback(() => {
    if (staged) {
      confirmUpload(inputRef.current);
    } else if (inputRef.current) {
      inputRef.current.click();
    }
  }, [staged]);

  const onFileChange = useCallback((e) => {
    if (e.target.files.length > 0) stageNewFile(e.target.files[0]);
  }, []);

  return html`
    <div class="view active">
      <${UploadProgress} />

      <div style="margin-bottom: 1.5rem; padding: 1.25rem; background: var(--surface); border-radius: 8px;">
        <label for="video-title" style="display: block; font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">
          Video Title (optional)
        </label>
        <input type="text" id="video-title" placeholder="Enter video title"
          style="width: 100%; padding: 0.6rem 0.75rem; background: var(--bg); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 0.9rem; margin-bottom: 0.75rem;" />

        <label for="video-price" style="display: block; font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">
          Price in SUI (0 = free)
        </label>
        <input type="number" id="video-price" placeholder="0" min="0" step="0.01"
          style="width: 100%; padding: 0.6rem 0.75rem; background: var(--bg); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 0.9rem; margin-bottom: 1rem;" />

        <${StagedFilePreview} />

        <div style="display:flex; align-items:center; justify-content:space-between;">
          <h2 style="margin-bottom:0;">Videos</h2>
          <div style="display:flex; gap:0.75rem; align-items:center;">
            <button class="btn" onclick=${handleUploadAction}>
              ${staged ? 'Confirm Upload' : 'Select Video'}
            </button>
            <input type="file" ref=${inputRef} accept="video/mp4,video/quicktime,video/webm,video/x-matroska,video/avi,.mp4,.mov,.webm,.mkv,.avi" style="display:none" onchange=${onFileChange} />
          </div>
        </div>
      </div>

      <div>
        ${loadError
          ? html`<div class="empty-state"><p>${loadError}</p></div>`
          : videos.length === 0
            ? html`<div class="empty-state"><p>No videos yet. Drag and drop a file or click Select MP4.</p></div>`
            : html`
                <div class="video-grid">
                  ${videos.map((v) => html`<${VideoCard} key=${v.id} video=${v} />`)}
                </div>
              `}
      </div>
    </div>
  `;
}
