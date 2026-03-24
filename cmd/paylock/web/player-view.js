import { html, useState, useEffect, useRef } from './lib.js';
import {
  viewParams, navGeneration, currentVideo, walletState,
  navigate, showToast, formatSui, loadWallet, setPollCleanup,
} from './state.js';

function ChainStatus({ video }) {
  if (!video) return null;

  if (video.sui_object_id) {
    const url = 'https://suiscan.xyz/testnet/object/' + video.sui_object_id + '/tx-blocks';
    const shortId = video.sui_object_id.slice(0, 10) + '...' + video.sui_object_id.slice(-4);
    return html`
      <div style="margin-top: 0.75rem; padding: 0.75rem 1rem; background: var(--surface); border-radius: 8px; font-size: 0.85rem;">
        <span style="color:var(--success);">On-chain</span>${' '}
        <a href=${url} target="_blank" rel="noopener noreferrer"
          style="font-family:monospace; color:var(--accent); font-size:0.75rem; text-decoration:none; border-bottom:1px dashed var(--accent);">
          ${shortId}
        </a>
        ${video.encrypted && html`
          <span style="color:var(--accent); font-size:0.75rem;"> Seal encrypted</span>
        `}
      </div>
    `;
  }

  if (video.price > 0 && !video.full_blob_id) {
    return html`
      <div style="margin-top: 0.75rem; padding: 0.75rem 1rem; background: var(--surface); border-radius: 8px; font-size: 0.85rem;">
        <span style="color:var(--warning);">Awaiting Seal encryption & on-chain publish</span>
      </div>
    `;
  }

  if (video.price > 0) {
    return html`
      <div style="margin-top: 0.75rem; padding: 0.75rem 1rem; background: var(--surface); border-radius: 8px; font-size: 0.85rem;">
        <span style="color:var(--text-muted);">Connect wallet to publish on-chain</span>
      </div>
    `;
  }

  return null;
}

function PaywallOverlay({ video, onPurchase, purchaseText, purchasing, hint, isOwner }) {
  if (!video) return null;
  return html`
    <div class="paywall-overlay" style="display: flex;">
      <div class="paywall-label">${isOwner ? 'Full video preview' : 'Preview ended'}</div>
      ${!isOwner && html`<div class="paywall-price">${formatSui(video.price)} SUI</div>`}
      <button class="btn" disabled=${purchasing} onclick=${onPurchase}>${purchaseText}</button>
      ${hint && html`<div class="paywall-label" style="font-size: 0.75rem; margin-top: 0.5rem;">${hint}</div>`}
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
    navigate('my-videos');
  } catch (err) {
    alert('Failed to delete video: ' + err.message);
  }
}

export function PlayerView() {
  const params = viewParams.value;
  const wallet = walletState.value;
  const gen = navGeneration.value;

  const [video, setVideo] = useState(null);
  const [status, setStatus] = useState('loading');
  const [statusText, setStatusText] = useState('');
  const [showPaywall, setShowPaywall] = useState(false);
  const [showLoading, setShowLoading] = useState(false);
  const [hint, setHint] = useState('');
  const [streamUrl, setStreamUrl] = useState('');
  const [purchaseText, setPurchaseText] = useState('Purchase & Unlock');
  const [purchasing, setPurchasing] = useState(false);
  const videoRef = useRef(null);

  // Load video status
  useEffect(() => {
    if (!params.id) return;
    let cancelled = false;
    let es = null;
    let interval = null;

    function cleanup() {
      cancelled = true;
      if (es) es.close();
      if (interval) clearInterval(interval);
    }
    setPollCleanup(cleanup);

    function onReady(v) {
      if (cancelled) return;
      setVideo(v);
      currentVideo.value = v;
      setStatus('ready');
      setShowLoading(false);
    }

    function onFailed(v) {
      if (cancelled) return;
      setStatus('failed');
      setShowLoading(false);
      setStatusText('Upload failed: ' + (v.error || 'unknown error'));
    }

    function startPolling() {
      interval = setInterval(async () => {
        if (cancelled) { clearInterval(interval); return; }
        try {
          const r = await fetch('/api/status/' + encodeURIComponent(params.id));
          if (cancelled) { clearInterval(interval); return; }
          if (!r.ok) return;
          const v = await r.json();
          if (v.status === 'ready') { clearInterval(interval); onReady(v); }
          else if (v.status === 'failed') { clearInterval(interval); onFailed(v); }
        } catch (_) { clearInterval(interval); }
      }, 1000);
    }

    async function load() {
      try {
        const res = await fetch('/api/status/' + encodeURIComponent(params.id));
        if (cancelled) return;
        if (!res.ok) { setStatus('not found'); return; }
        const data = await res.json();
        if (cancelled) return;

        if (data.status === 'ready') {
          onReady(data);
        } else if (data.status === 'processing') {
          setShowLoading(true);
          setStatus('processing');

          if (typeof EventSource !== 'undefined') {
            es = new EventSource('/api/status/' + encodeURIComponent(params.id) + '/events');
            es.onmessage = (e) => {
              if (cancelled) { es.close(); return; }
              const v = JSON.parse(e.data);
              if (v.status === 'ready') { es.close(); onReady(v); }
              else if (v.status === 'failed') { es.close(); onFailed(v); }
            };
            es.onerror = () => { es.close(); startPolling(); };
          } else {
            startPolling();
          }
        } else {
          setStatus('failed');
          setStatusText('Upload failed: ' + (data.error || 'unknown error'));
        }
      } catch (_) {
        if (!cancelled) { setStatus('error'); setStatusText('Cannot connect to server.'); }
      }
    }

    load();
    return cleanup;
  }, [params.id, gen]);

  // Playback logic when video becomes ready
  useEffect(() => {
    if (!video || status !== 'ready') return;
    const el = videoRef.current;
    if (!el) return;

    // Free video: play full blob directly
    if (video.price === 0 && video.full_blob_url) {
      setStreamUrl(video.full_blob_url);
      el.src = video.full_blob_url;
      el.play().catch(() => {});
      return;
    }

    // Paid video with wallet connected: try auto-decrypt (recover full_blob_url from chain if missing)
    if (video.price > 0 && video.encrypted && video.sui_object_id && wallet.connected) {
      tryAutoDecrypt(video, el);
      return;
    }

    // Default: play preview
    playPreview(video, el);
  }, [video, status, wallet.connected]);

  function isOwner(v) {
    return v.creator && wallet.address && v.creator === wallet.address;
  }

  async function tryAutoDecrypt(v, el) {
    try {
      const mod = await loadWallet();
      const ownerMode = isOwner(v);

      if (!ownerMode) {
        const passId = await mod.findAccessPass(v.sui_object_id);
        if (!passId) { playPreview(v, el); return; }
      }

      // Recover full_blob_url from chain if missing
      if (!v.full_blob_url) {
        setStreamUrl('Recovering blob ID from chain...');
        await mod.recoverFullBlobId(v);
        const res = await fetch('/api/status/' + encodeURIComponent(v.id));
        if (!res.ok) { playPreview(v, el); return; }
        v = await res.json();
        setVideo(v);
      }

      if (!v.full_blob_url) { playPreview(v, el); return; }

      setStreamUrl('Decrypting full video...');
      const blobUrl = ownerMode
        ? await mod.decryptVideoAsOwner(v)
        : await mod.decryptVideo(v);
      revokeOldBlob(el);
      el.onended = null;
      setShowPaywall(false);
      setStreamUrl(blobUrl);
      el.src = blobUrl;
      el.play().catch(() => {});
    } catch (err) {
      setHint('Auto-decrypt failed: ' + err.message);
      playPreview(v, el);
    }
  }

  function playPreview(v, el) {
    if (v.preview_blob_url) {
      setStreamUrl(v.preview_blob_url);
      el.src = v.preview_blob_url;
      el.play().catch(() => {});
    }
    if (v.price > 0) {
      el.onended = () => {
        setShowPaywall(true);
        if (!wallet.connected) setHint('Connect wallet to purchase');
        else if (!v.sui_object_id) setHint('Video not yet published on-chain');
        else if (isOwner(v)) { setHint('You own this video'); setPurchaseText('Unlock'); }
        else setHint('');
      };
    }
  }

  function revokeOldBlob(el) {
    if (el.src && el.src.startsWith('blob:')) URL.revokeObjectURL(el.src);
  }

  async function handlePurchase() {
    if (!wallet.connected) {
      const mod = await loadWallet();
      mod.connectWallet();
      return;
    }
    if (!video || !video.sui_object_id) {
      setHint('Video not published on-chain yet');
      return;
    }

    setPurchasing(true);
    setHint('');

    try {
      const mod = await loadWallet();
      const el = videoRef.current;

      const ownerMode = isOwner(video);

      let accessPassId = null;
      if (!ownerMode) {
        setPurchaseText('Checking access...');
        accessPassId = await mod.findAccessPass(video.sui_object_id);

        if (!accessPassId) {
          setPurchaseText('Purchasing...');
          accessPassId = await mod.purchaseVideo(video);
        }
      }

      if (!video.full_blob_url && video.sui_object_id) {
        setPurchaseText('Recovering blob ID from chain...');
        try {
          const fullBlobId = await mod.recoverFullBlobId(video);
          const res = await fetch('/api/status/' + encodeURIComponent(video.id));
          if (res.ok) {
            const refreshed = await res.json();
            setVideo(refreshed);
            video = refreshed;
          }
        } catch (recoverErr) {
          setHint('Recovery failed: ' + recoverErr.message);
          setPurchasing(false);
          setPurchaseText('Purchase & Unlock');
          return;
        }
      }

      if (!video.full_blob_url) {
        setHint('Encrypted blob not available — the upload may have been interrupted');
        setPurchasing(false);
        setPurchaseText('Purchase & Unlock');
        return;
      }

      setPurchaseText('Decrypting...');
      const blobUrl = ownerMode
        ? await mod.decryptVideoAsOwner(video)
        : await mod.decryptVideo(video, accessPassId);

      revokeOldBlob(el);
      el.onended = null;
      setShowPaywall(false);
      setStreamUrl(blobUrl);
      el.src = blobUrl;
      el.play().catch(() => {});
    } catch (err) {
      setPurchasing(false);
      setPurchaseText('Purchase & Unlock');
      setHint('Failed: ' + err.message);
    }
  }

  function copyUrl() {
    navigator.clipboard.writeText(streamUrl).then(() => {
      showToast('success', 'Copied!');
    });
  }

  const safeStatus = ['ready', 'processing', 'failed'].includes(status) ? status : 'failed';
  const title = video ? (video.title || video.id) : (params.id || '');

  return html`
    <div class="view active">
      <a class="back-link" onclick=${() => navigate('my-videos')}>\u2190 Back to videos</a>
      <div class="player-info">
        <div style="display:flex; flex-direction:column;">
          <span style="font-size:1.25rem; font-weight:600; margin-bottom:0.25rem;">${title}</span>
          <span style="font-size:0.8rem; color:var(--text-muted); font-family:monospace;">${params.id}</span>
        </div>
        ${status !== 'loading' && html`<span class=${'status-badge ' + safeStatus}>${status}</span>`}
      </div>

      <div class="player-container" style="position: relative;">
        <video ref=${videoRef} controls></video>

        ${showLoading && html`
          <div class="player-loading-overlay" style="display: flex;">
            <div class="player-loading-spinner"></div>
            <div class="player-loading-text">Uploading to Walrus...</div>
          </div>
        `}

        ${showPaywall && html`
          <${PaywallOverlay}
            video=${video}
            onPurchase=${handlePurchase}
            purchaseText=${purchaseText}
            purchasing=${purchasing}
            hint=${hint}
            isOwner=${video && isOwner(video)}
          />
        `}

        ${video && html`<${ChainStatus} video=${video} />`}
      </div>

      <div class="stream-url">${streamUrl || statusText}</div>
      <div class="player-actions">
        <button class="btn btn-sm btn-outline" onclick=${copyUrl}>Copy URL</button>
        <button class="btn btn-sm btn-danger" onclick=${() => params.id && deleteVideo(params.id)}>Delete</button>
      </div>
    </div>
  `;
}
