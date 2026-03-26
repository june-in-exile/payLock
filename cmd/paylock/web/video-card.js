import { html } from './lib.js';
import { navigate, formatDate, formatSui } from './state.js';
import { signForAuth, setAuthHeaders, isWalletConnected } from './wallet.js';

export async function deleteVideo(id, onDeleted) {
  if (!confirm('Are you sure you want to delete this video? This action cannot be undone.')) return;
  try {
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
    if (onDeleted) onDeleted();
  } catch (err) {
    alert('Failed to delete video: ' + err.message);
  }
}

function shortAddr(addr) {
  if (!addr || addr.length < 12) return addr || '';
  return addr.slice(0, 6) + '...' + addr.slice(-4);
}

export function VideoCard({ video, showDelete, onDeleted, accessState }) {
  const safeStatus = ['ready', 'processing', 'failed'].includes(video.status) ? video.status : 'failed';
  const isPaid = video.price > 0;
  const safeAccess = ['locked', 'unlocked', 'checking'].includes(accessState) ? accessState : null;
  const accessLabel = safeAccess === 'checking'
    ? 'Checking'
    : safeAccess === 'unlocked'
      ? 'Unlocked'
      : safeAccess === 'locked'
        ? 'Locked'
        : '';

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
          ${video.sui_object_id
            ? html`<span>Object ID: <a href=${'https://suiscan.xyz/testnet/object/' + video.sui_object_id} target="_blank" rel="noopener noreferrer"
                onclick=${(e) => e.stopPropagation()}
                style="font-family:monospace; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
                title=${video.sui_object_id}>${shortAddr(video.sui_object_id)}</a></span>`
            : html`<span style="font-family:monospace">ID: ${video.id}</span>`
          }
          <span class=${'status-badge ' + safeStatus}>${video.status}</span>
          <span class=${isPaid ? 'price-badge paid' : 'price-badge free'}>
            ${isPaid ? formatSui(video.price) + ' SUI' : 'Free'}
          </span>
          ${isPaid && safeAccess && html`
            <span class=${'access-badge ' + safeAccess}>${accessLabel}</span>
          `}
          <span>${formatDate(video.created_at)}</span>
          ${video.creator && html`
            <span>Owner: <a href=${'https://suiscan.xyz/testnet/account/' + video.creator} target="_blank" rel="noopener noreferrer"
              onclick=${(e) => e.stopPropagation()}
              style="font-family:monospace; color:var(--accent); text-decoration:none; border-bottom:1px dashed var(--accent);"
              title=${video.creator}>
              ${shortAddr(video.creator)}
            </a></span>
          `}
        </div>
      </div>
      ${showDelete && html`
        <div class="video-actions">
          <button class="btn btn-sm btn-danger" onclick=${(e) => { e.stopPropagation(); deleteVideo(video.id, onDeleted); }}>
            Delete
          </button>
        </div>
      `}
    </div>
  `;
}
