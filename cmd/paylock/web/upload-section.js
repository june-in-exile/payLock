import { html, useRef, useCallback } from './lib.js';
import {
  stagedFile, uploadState, navigate, showToast,
  formatFileSize, stageNewFile, clearStagedFile,
  loadWallet, walletState,
} from './state.js';
import { signForAuth, setAuthHeaders, isWalletConnected } from './wallet.js';

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

function StagedFilePreview({ inputRef }) {
  const staged = stagedFile.value;

  if (!staged) return null;

  return html`
    <div class="staged-file active">
      <div class="staged-file-preview">
        <video src=${staged.objectUrl} muted></video>
      </div>
      <div class="staged-file-info">
        <div class="staged-file-name">${staged.name}</div>
        <div class="staged-file-size">${formatFileSize(staged.size)}</div>
        <div class="staged-file-actions">
          <button class="btn btn-outline" onclick=${() => inputRef.current && inputRef.current.click()}>
            Change File
          </button>
          <button class="btn btn-outline" onclick=${() => { clearStagedFile(); videoDuration = null; updatePreviewControls(); }}>Remove</button>
        </div>
      </div>
    </div>
  `;
}

function sendUpload(formData, fileName, authHeaders) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/upload');

    if (authHeaders) {
      for (const [k, v] of Object.entries(authHeaders)) {
        xhr.setRequestHeader(k, v);
      }
    }

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

let previewDurationDefault = 10;
let previewDurationMin = 10;
let previewDurationMax = 30;
let serverMaxPreviewDuration = 300;
let videoDuration = null;
let previewDurationSec = previewDurationDefault;
let previewControlsScheduled = false;

function effectiveMax() {
  if (videoDuration !== null) {
    const floored = Math.floor(videoDuration);
    return Math.min(serverMaxPreviewDuration, floored);
  }
  return serverMaxPreviewDuration;
}

function updatePreviewControls() {
  const range = document.getElementById('preview-duration-range');
  const number = document.getElementById('preview-duration');
  const hint = document.getElementById('preview-duration-hint');

  previewDurationMax = effectiveMax();
  if (previewDurationSec > previewDurationMax) previewDurationSec = previewDurationMax;
  if (previewDurationSec < previewDurationMin) previewDurationSec = previewDurationMin;

  if (range) {
    range.min = previewDurationMin;
    range.max = previewDurationMax;
    range.value = previewDurationSec;
  }
  if (number) {
    number.min = previewDurationMin;
    number.max = previewDurationMax;
    number.value = previewDurationSec;
  }
  if (hint) {
    const parts = [`Default ${previewDurationDefault}s`, `Min ${previewDurationMin}s`, `Max ${previewDurationMax}s`];
    if (videoDuration !== null) {
      parts.push(`Video ${Math.floor(videoDuration)}s`);
    }
    hint.textContent = parts.join(' • ');
  }
}

async function fetchPreviewConfig() {
  try {
    const res = await fetch('/api/config');
    if (res.ok) {
      const cfg = await res.json();
      if (cfg.preview_duration_default > 0) previewDurationDefault = cfg.preview_duration_default;
      if (cfg.preview_duration_min > 0) previewDurationMin = cfg.preview_duration_min;
      if (cfg.preview_duration_max > 0) {
        serverMaxPreviewDuration = cfg.preview_duration_max;
        previewDurationMax = cfg.preview_duration_max;
      }
      if (!cfg.preview_duration_default && cfg.preview_duration > 0) {
        previewDurationDefault = cfg.preview_duration;
      }
      previewDurationSec = previewDurationDefault;
      updatePreviewControls();
    }
  } catch (_) {
    // fallback to default
  }
}

fetchPreviewConfig();

export function detectVideoDuration(file) {
  const video = document.createElement('video');
  video.preload = 'metadata';
  const url = URL.createObjectURL(file);
  video.src = url;

  function tryRead() {
    if (Number.isFinite(video.duration) && video.duration > 0) {
      videoDuration = video.duration;
      URL.revokeObjectURL(url);
      updatePreviewControls();
      return true;
    }
    return false;
  }

  video.addEventListener('loadedmetadata', () => {
    if (!tryRead()) {
      // MOV/some formats report Infinity initially; wait for durationchange
      video.addEventListener('durationchange', () => {
        if (tryRead()) return;
        // Still not finite — give up
        URL.revokeObjectURL(url);
        videoDuration = null;
        updatePreviewControls();
      }, { once: true });
    }
  });
  video.addEventListener('error', () => {
    URL.revokeObjectURL(url);
    videoDuration = null;
    updatePreviewControls();
  });
}

function readPreviewDuration() {
  const input = document.getElementById('preview-duration');
  if (!input) return previewDurationDefault;
  const sec = parseInt(input.value, 10);
  if (Number.isNaN(sec)) return null;
  return sec;
}

function generatePreview(file) {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video');
    video.muted = true;
    video.playsInline = true;
    video.preload = 'auto';

    const objectUrl = URL.createObjectURL(file);
    video.src = objectUrl;

    const cleanup = () => URL.revokeObjectURL(objectUrl);

    video.addEventListener('error', () => {
      cleanup();
      reject(new Error('Failed to load video for preview generation'));
    });

    video.addEventListener('loadedmetadata', () => {
      const clipDuration = Math.min(video.duration, previewDurationSec);

      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const ctx = canvas.getContext('2d');

      const stream = canvas.captureStream();
      const audioTracks = video.captureStream ? video.captureStream().getAudioTracks() : [];
      for (const track of audioTracks) stream.addTrack(track);

      const mimeType = MediaRecorder.isTypeSupported('video/webm;codecs=vp9,opus')
        ? 'video/webm;codecs=vp9,opus'
        : 'video/webm';
      const recorder = new MediaRecorder(stream, { mimeType });
      const chunks = [];

      recorder.ondataavailable = (e) => { if (e.data.size > 0) chunks.push(e.data); };

      recorder.onstop = () => {
        cleanup();
        const blob = new Blob(chunks, { type: mimeType });
        const previewFile = new File([blob], 'preview.webm', { type: mimeType });
        resolve(previewFile);
      };

      recorder.onerror = () => {
        cleanup();
        reject(new Error('Preview recording failed'));
      };

      video.addEventListener('seeked', () => {
        video.play();
        recorder.start();

        const drawFrame = () => {
          if (video.currentTime >= clipDuration || video.ended) {
            recorder.stop();
            video.pause();
            return;
          }
          ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
          requestAnimationFrame(drawFrame);
        };
        requestAnimationFrame(drawFrame);
      }, { once: true });

      video.currentTime = 0;
    });
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

function pollUntilPreviewUploaded(id) {
  return new Promise((resolve, reject) => {
    if (typeof EventSource !== 'undefined') {
      const es = new EventSource('/api/status/' + encodeURIComponent(id) + '/events');
      es.onmessage = (e) => {
        const video = JSON.parse(e.data);
        if (video.status === 'failed') { es.close(); reject(new Error(video.error || 'Upload failed')); }
        else if (video.preview_blob_id) { es.close(); resolve(video); }
      };
      es.onerror = () => {
        es.close();
        const interval = setInterval(async () => {
          try {
            const res = await fetch('/api/status/' + encodeURIComponent(id));
            if (!res.ok) return;
            const video = await res.json();
            if (video.status === 'failed') { clearInterval(interval); reject(new Error(video.error || 'Upload failed')); }
            else if (video.preview_blob_id) { clearInterval(interval); resolve(video); }
          } catch (err) { clearInterval(interval); reject(err); }
        }, 1000);
      };
      return;
    }
  });
}

async function confirmUpload(fileInput) {
  const staged = stagedFile.value;
  if (!staged) return;

  const previewDuration = readPreviewDuration();
  if (previewDuration === null || previewDuration < previewDurationMin || previewDuration > previewDurationMax) {
    showToast('error', `Preview length must be between ${previewDurationMin}s and ${previewDurationMax}s.`, 5000);
    return;
  }
  previewDurationSec = previewDuration;

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

  let previewFile = null;
  if (priceMist > 0) {
    uploadState.value = { active: true, percent: 0, text: 'Generating preview clip...', step: null, showSpinner: true, showSteps: false };
    try {
      previewFile = await generatePreview(file);
    } catch (err) {
      uploadState.value = { active: false, percent: 0, text: '', step: null, showSpinner: false, showSteps: false };
      showToast('error', 'Failed to generate preview: ' + err.message, 5000);
      return;
    }
  }

  uploadState.value = { active: true, percent: 0, text: 'Uploading ' + file.name + '...', step: null, showSpinner: false, showSteps: false };

  const formData = new FormData();
  formData.append(priceMist > 0 ? 'preview' : 'video', previewFile || file);
  formData.append('preview_duration', previewDurationSec.toString());
  const title = document.getElementById('video-title').value;
  if (title) formData.append('title', title);
  if (priceMist > 0) formData.append('price', priceMist.toString());

  let authHeaders = null;
  if (isWalletConnected()) {
    const auth = await signForAuth('upload', '');
    authHeaders = {};
    setAuthHeaders(authHeaders, auth);
  }

  let data = null;
  try {
    data = await sendUpload(formData, file.name, authHeaders);

    let navigateId = data.id;

    if (priceMist > 0 && fileArrayBuffer) {
      uploadState.value = { ...uploadState.value, showSpinner: true, showSteps: true, step: 'parallel', previewDone: false, browserStep: 'encrypt', text: 'Encrypting & uploading...' };

      const mod = await loadWallet();

      const [video, encResult] = await Promise.all([
        pollUntilPreviewUploaded(data.id).then((v) => {
          uploadState.value = { ...uploadState.value, previewDone: true };
          return v;
        }),
        mod.encryptAndPublish(fileArrayBuffer, (browserStep) => {
          uploadState.value = { ...uploadState.value, browserStep };
          if (browserStep === 'walrus') uploadState.value = { ...uploadState.value, text: 'Uploading encrypted blob to Walrus...' };
        }),
      ]);

      uploadState.value = { ...uploadState.value, step: 'onchain', previewDone: true, browserStep: 'done', text: 'Creating video on-chain...' };
      const suiObjectId = await mod.createVideoOnChain(data.id, priceMist, video.preview_blob_id, encResult.fullBlobId, encResult.namespace, data.session_token);
      navigateId = suiObjectId;
    } else {
      uploadState.value = { ...uploadState.value, showSpinner: true, text: 'Processing video...' };
      const video = await pollUntilReady(data.id);
      if (video.sui_object_id) navigateId = video.sui_object_id;
    }

    clearStagedFile();
    videoDuration = null;
    updatePreviewControls();
    if (fileInput) fileInput.value = '';
    document.getElementById('video-title').value = '';
    document.getElementById('video-price').value = '';
    uploadState.value = { active: false, percent: 0, text: '', step: null, showSpinner: false, showSteps: false };
    showToast('success', 'Upload complete!');
    navigate('player', { id: navigateId });
  } catch (err) {
    // Paid upload failed after preview was already uploaded — clean up the orphaned video
    if (priceMist > 0 && data) {
      try {
        await fetch('/api/videos/' + encodeURIComponent(data.id), { method: 'DELETE' });
      } catch (_) { /* best-effort cleanup */ }
    }
    uploadState.value = { active: false, percent: 0, text: '', step: null, showSpinner: false, showSteps: false };
    showToast('error', 'Upload failed: ' + err.message, 5000);
  }
}

export function UploadSection() {
  const inputRef = useRef(null);
  const staged = stagedFile.value;

  if (!previewControlsScheduled) {
    previewControlsScheduled = true;
    setTimeout(updatePreviewControls, 0);
  }

  setTimeout(updatePreviewControls, 0);

  const handleUploadAction = useCallback(() => {
    if (uploadState.value.active) return;
    if (staged) {
      confirmUpload(inputRef.current);
    } else if (inputRef.current) {
      inputRef.current.click();
    }
  }, [staged]);

  const onFileChange = useCallback((e) => {
    if (e.target.files.length > 0) {
      stageNewFile(e.target.files[0]);
      detectVideoDuration(e.target.files[0]);
    }
  }, []);

  const onPreviewRangeInput = useCallback((e) => {
    const val = parseInt(e.target.value, 10);
    if (Number.isNaN(val)) return;
    previewDurationSec = val;
    const number = document.getElementById('preview-duration');
    if (number) number.value = val;
  }, []);

  const onPreviewNumberInput = useCallback((e) => {
    const val = parseInt(e.target.value, 10);
    if (Number.isNaN(val)) return;
    previewDurationSec = val;
    const range = document.getElementById('preview-duration-range');
    if (range) range.value = val;
  }, []);

  return html`
    <div>
      <${UploadProgress} />

      <div style="margin-bottom: 1.5rem; padding: 1.25rem; background: var(--surface); border-radius: 8px;">
        <label for="video-title" style="display: block; font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">
          Video Title (optional)
        </label>
        <input type="text" id="video-title" placeholder="Enter video title"
          class="form-input" style="margin-bottom: 0.75rem;" />

        <label for="video-price" style="display: block; font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">
          Price in SUI (0 = free)
        </label>
        <input type="number" id="video-price" placeholder="0" min="0" step="0.01"
          class="form-input" style="margin-bottom: 1rem;" />

        <label for="preview-duration" style="display: block; font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem;">
          Preview length (seconds)
        </label>
        <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.35rem;">
          <input
            type="range"
            id="preview-duration-range"
            min=${previewDurationMin}
            max=${previewDurationMax}
            value=${previewDurationDefault}
            oninput=${onPreviewRangeInput}
            style="flex: 1;"
          />
          <input
            type="number"
            id="preview-duration"
            min=${previewDurationMin}
            max=${previewDurationMax}
            value=${previewDurationDefault}
            class="form-input"
            oninput=${onPreviewNumberInput}
            style="width: 6rem;"
          />
        </div>
        <div id="preview-duration-hint" style="font-size: 0.75rem; color: var(--text-muted); margin-bottom: 1rem;">
          Default ${previewDurationDefault}s • Min ${previewDurationMin}s • Max ${previewDurationMax}s
        </div>

        <${StagedFilePreview} inputRef=${inputRef} />

        <div style="display:flex; align-items:center; justify-content:flex-end;">
          <div style="display:flex; gap:0.75rem; align-items:center;">
            <button class="btn" onclick=${handleUploadAction} disabled=${uploadState.value.active}>
              ${staged ? 'Confirm Upload' : 'Select Video'}
            </button>
            <input type="file" ref=${inputRef} accept="video/mp4,video/quicktime,video/webm,video/x-matroska,video/avi,.mp4,.mov,.webm,.mkv,.avi" style="display:none" onchange=${onFileChange} />
          </div>
        </div>
      </div>
    </div>
  `;
}
