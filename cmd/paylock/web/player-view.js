import { html, useState, useEffect, useRef } from './lib.js';
import {
  viewParams, navGeneration, currentVideo, walletState,
  navigate, formatSui, formatDate, loadWallet, setPollCleanup,
} from './state.js';
import { signForAuth, setAuthHeaders, isWalletConnected } from './wallet.js';

function DetailRow({ label, children }) {
  return html`
    <div class="detail-row">
      <span class="detail-label">${label}</span>
      <span class="detail-value">${children}</span>
    </div>
  `;
}

function VideoDetails({ video, hasAccess, wallet }) {
  if (!video) return null;

  const isPaid = video.price > 0;
  const ownerMode = video.creator && wallet.address && video.creator === wallet.address;

  return html`
    <div class="video-details">
      <div class="video-details-title">Details</div>

      <${DetailRow} label="Status">
        <span class=${'status-badge ' + video.status}>${video.status}</span>
      <//>

      <${DetailRow} label="Type">
        <span class=${isPaid ? 'price-badge paid' : 'price-badge free'}>
          ${isPaid ? formatSui(video.price) + ' SUI' : 'Free'}
        </span>
        ${video.encrypted && html`
          <span style="margin-left:0.4rem; font-size:0.75rem; color:var(--accent);">Seal encrypted</span>
        `}
      <//>

      ${isPaid && wallet.connected && html`
        <${DetailRow} label="Access">
          ${ownerMode
            ? html`<span class="access-badge unlocked">Owner</span>`
            : hasAccess
              ? html`<span class="access-badge unlocked">Unlocked</span>`
              : html`<span class="access-badge locked">Locked</span>`
          }
        <//>
      `}

      <${DetailRow} label="Uploaded">
        ${formatDate(video.created_at)}
      <//>

      ${video.creator && html`
        <${DetailRow} label="Creator">
          <a href=${'https://suiscan.xyz/testnet/account/' + video.creator} target="_blank" rel="noopener noreferrer"
            style="font-family:monospace; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
            title=${video.creator}>${video.creator.slice(0, 10) + '...' + video.creator.slice(-4)}</a>
        <//>
      `}

      ${video.sui_object_id && html`
        <${DetailRow} label="Object ID">
          <a href=${'https://suiscan.xyz/testnet/object/' + video.sui_object_id} target="_blank" rel="noopener noreferrer"
            style="font-family:monospace; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
            title=${video.sui_object_id}>${video.sui_object_id.slice(0, 10) + '...' + video.sui_object_id.slice(-4)}</a>
        <//>
      `}

      ${!video.sui_object_id && html`
        <${DetailRow} label="Paylock ID">
          <span style="font-family:monospace;">${video.id}</span>
        <//>
      `}

      ${video.preview_blob_id && html`
        <${DetailRow} label="Preview Blob">
          <a href=${'https://walruscan.com/testnet/blob/' + video.preview_blob_id} target="_blank" rel="noopener noreferrer"
            style="font-family:monospace; font-size:0.75rem; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
            title=${video.preview_blob_id}>${video.preview_blob_id.slice(0, 12) + '...'}</a>
        <//>
      `}

      ${video.full_blob_id && html`
        <${DetailRow} label="Full Blob">
          <a href=${'https://walruscan.com/testnet/blob/' + video.full_blob_id} target="_blank" rel="noopener noreferrer"
            style="font-family:monospace; font-size:0.75rem; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
            title=${video.full_blob_id}>${video.full_blob_id.slice(0, 12) + '...'}</a>
        <//>
      `}

      ${video.deleted && html`
        <${DetailRow} label="Deleted">
          <span style="color:var(--error);">${formatDate(video.deleted_at) || 'Yes'}</span>
        <//>
      `}

      ${video.error && html`
        <${DetailRow} label="Error">
          <span style="color:var(--error);">${video.error}</span>
        <//>
      `}
    </div>
  `;
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

async function deleteVideo(id, suiObjectId) {
  try {
    // If the video is on-chain, delete it from the chain first.
    if (suiObjectId && isWalletConnected()) {
      const mod = await loadWallet();
      await mod.deleteVideoOnChain(suiObjectId);
    }

    const headers = {};
    if (isWalletConnected()) {
      const auth = await signForAuth('delete', id);
      setAuthHeaders(headers, auth);
    }
    const res = await fetch('/api/videos/' + encodeURIComponent(id), { method: 'DELETE', headers });
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
  const [purchaseText, setPurchaseText] = useState('Unlock');
  const [purchasing, setPurchasing] = useState(false);
  const [hasAccess, setHasAccess] = useState(false);
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
          const r = await fetch('/api/videos/' + encodeURIComponent(params.id));
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
        const res = await fetch('/api/videos/' + encodeURIComponent(params.id));
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
            es = new EventSource('/api/videos/' + encodeURIComponent(params.id));
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
      el.src = video.full_blob_url;
      el.muted = true;
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

  // Check access pass when wallet/video changes
  useEffect(() => {
    let cancelled = false;

    async function checkAccess() {
      if (!video || video.price === 0 || !video.sui_object_id || !wallet.connected) {
        if (!cancelled) setHasAccess(false);
        return;
      }
      if (isOwner(video)) {
        if (!cancelled) setHasAccess(true);
        return;
      }
      try {
        const mod = await loadWallet();
        const passId = await mod.findAccessPass(video.sui_object_id);
        if (!cancelled) setHasAccess(!!passId);
      } catch (_) {
        if (!cancelled) setHasAccess(false);
      }
    }

    checkAccess();
    return () => { cancelled = true; };
  }, [video, wallet.connected, wallet.address]);

  // Keep purchase text aligned with access state when idle
  useEffect(() => {
    if (purchasing) return;
    if (hasAccess) setPurchaseText('Unlock');
    else setPurchaseText('Unlock');
  }, [hasAccess, purchasing]);

  async function tryAutoDecrypt(v, el) {
    try {
      const mod = await loadWallet();
      const ownerMode = isOwner(v);

      if (!ownerMode) {
        const passId = await mod.findAccessPass(v.sui_object_id);
        if (!passId) { playPreview(v, el); return; }
        setHasAccess(true);
      }

      if (!v.full_blob_url) {
        const res = await fetch('/api/videos/' + encodeURIComponent(v.id));
        if (!res.ok) { setError('Failed to load video details'); return; }
        v = await res.json();
        setVideo(v);
      }

      if (!v.full_blob_url) { playPreview(v, el); return; }

      const blobUrl = ownerMode
        ? await mod.decryptVideoAsOwner(v)
        : await mod.decryptVideo(v);
      revokeOldBlob(el);
      el.onended = null;
      setShowPaywall(false);
      el.src = blobUrl;
      el.muted = true;
      el.play().catch(() => {});
    } catch (_) {
      // Silently fall back to preview — user didn't request decrypt yet.
      // If they click Unlock, handlePurchase will show errors explicitly.
      playPreview(v, el);
    }
  }

  function playPreview(v, el) {
    if (v.preview_blob_url) {
      el.src = v.preview_blob_url;
      el.muted = true;
      el.play().catch(() => {});
    }
    if (v.price > 0) {
      el.onended = () => {
        setShowPaywall(true);
        if (!wallet.connected) setHint('Connect wallet to purchase');
        else if (!v.sui_object_id) setHint('Video not yet published on-chain');
        else if (isOwner(v)) { setHint('You own this video'); setPurchaseText('Unlock'); }
        else if (hasAccess) { setHint('Access pass found'); setPurchaseText('Unlock'); }
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
      if (accessPassId || ownerMode) setHasAccess(true);

      if (!video.full_blob_url && video.sui_object_id) {
        setPurchaseText('Recovering blob ID from chain...');
        try {
          const res = await fetch('/api/videos/' + encodeURIComponent(video.id));
          if (!res.ok) {
            setHint('Failed to load video details');
            setPurchasing(false);
            setPurchaseText('Unlock');
            return;
          }
          const refreshed = await res.json();
          setVideo(refreshed);
          video = refreshed;
        } catch (recoverErr) {
          setHint('Recovery failed: ' + recoverErr.message);
          setPurchasing(false);
          setPurchaseText('Unlock');
          return;
        }
      }

      if (!video.full_blob_url) {
        setHint('Encrypted blob not available — the upload may have been interrupted');
        setPurchasing(false);
        setPurchaseText('Unlock');
        return;
      }

      setPurchaseText('Decrypting...');
      const blobUrl = ownerMode
        ? await mod.decryptVideoAsOwner(video)
        : await mod.decryptVideo(video, accessPassId);

      revokeOldBlob(el);
      el.onended = null;
      setShowPaywall(false);
      el.src = blobUrl;
      el.muted = true;
      el.play().catch(() => {});
      setPurchasing(false);
    } catch (err) {
      setPurchasing(false);
      setPurchaseText('Unlock');
      setHint('Failed: ' + err.message);
    }
  }

  const safeStatus = ['ready', 'processing', 'failed'].includes(status) ? status : 'failed';
  const title = video ? (video.title || video.id) : (params.id || '');

  return html`
    <div class="view active">
      <a class="back-link" onclick=${() => navigate('my-videos')}>\u2190 Back to videos</a>
      <div class="player-info">
        <div style="display:flex; align-items:center; gap:0.6rem; flex-wrap:wrap;">
          <span style="font-size:1.25rem; font-weight:600;">${title}</span>
          ${status !== 'loading' && html`<span class=${'status-badge ' + safeStatus}>${status}</span>`}
        </div>
      </div>

      <div class="player-container" style="position: relative;">
        <video ref=${videoRef} controls muted autoplay></video>

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

        ${!showPaywall && !showLoading && video && video.price > 0
          && video.sui_object_id && status === 'ready' && !isOwner(video) && !hasAccess && html`
          <button
            class="btn early-purchase-btn"
            disabled=${purchasing}
            onclick=${handlePurchase}
          >
            ${purchasing ? purchaseText : `${formatSui(video.price)} SUI Unlock`}
          </button>
        `}

        ${''}
      </div>

      ${statusText && html`
        <div style="font-size: 0.85rem; color: var(--text-muted); background: var(--surface); padding: 0.6rem 0.8rem; border-radius: 6px; margin-bottom: 1rem; border: 1px solid var(--border);">
          ${statusText}
        </div>
      `}

      ${video && html`<${VideoDetails} video=${video} hasAccess=${hasAccess} wallet=${wallet} />`}

      <div class="player-actions">
        ${video && isOwner(video) && html`
          <button class="btn btn-sm btn-danger" onclick=${() => params.id && deleteVideo(params.id, video && video.sui_object_id)}>Delete</button>
        `}
      </div>
    </div>
  `;
}
