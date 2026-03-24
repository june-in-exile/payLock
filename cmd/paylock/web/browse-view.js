import { html, useState, useEffect } from './lib.js';
import { navGeneration, walletState, loadWallet } from './state.js';
import { VideoCard } from './video-card.js';

function accessStateForVideo(video, wallet, accessMap) {
  if (!video || video.price === 0) return null;
  if (wallet.connected && wallet.address && video.creator === wallet.address) return 'unlocked';
  if (!wallet.connected || !wallet.address) return 'locked';
  if (!video.sui_object_id) return 'locked';
  const key = video.sui_object_id || video.id;
  return accessMap[key] || 'checking';
}

export function BrowseView() {
  const [videos, setVideos] = useState([]);
  const [loadError, setLoadError] = useState(null);
  const [accessMap, setAccessMap] = useState({});
  const wallet = walletState.value;
  const generation = navGeneration.value;

  useEffect(() => {
    let cancelled = false;

    fetch('/api/videos')
      .then((res) => {
        if (!res.ok) throw new Error('Failed to load');
        return res.json();
      })
      .then((data) => { if (!cancelled) { setVideos(data.videos || []); setLoadError(null); } })
      .catch(() => { if (!cancelled) setLoadError('Cannot connect to server.'); });

    return () => { cancelled = true; };
  }, [generation]);

  useEffect(() => {
    let cancelled = false;

    if (!wallet.connected || !wallet.address || videos.length === 0) {
      setAccessMap({});
      return () => { cancelled = true; };
    }

    const candidates = videos.filter((v) => (
      v.price > 0
      && v.sui_object_id
      && !(v.creator && v.creator === wallet.address)
    ));

    if (candidates.length === 0) {
      setAccessMap({});
      return () => { cancelled = true; };
    }

    setAccessMap((prev) => {
      const next = { ...prev };
      for (const v of candidates) {
        const key = v.sui_object_id || v.id;
        if (!next[key]) next[key] = 'checking';
      }
      return next;
    });

    (async () => {
      try {
        const mod = await loadWallet();
        for (const v of candidates) {
          if (cancelled) return;
          const key = v.sui_object_id || v.id;
          let unlocked = false;
          try {
            const passId = await mod.findAccessPass(v.sui_object_id);
            unlocked = !!passId;
          } catch (_) {
            unlocked = false;
          }
          if (!cancelled) {
            setAccessMap((prev) => ({
              ...prev,
              [key]: unlocked ? 'unlocked' : 'locked',
            }));
          }
        }
      } catch (_) {
        // Ignore wallet loading errors for browse access labels.
      }
    })();

    return () => { cancelled = true; };
  }, [videos, wallet.connected, wallet.address]);

  return html`
    <div class="view active">
      <h2 style="margin-bottom: 1rem;">Browse Videos</h2>
      <div>
        ${loadError
          ? html`<div class="empty-state"><p>${loadError}</p></div>`
          : videos.length === 0
            ? html`<div class="empty-state"><p>No videos available yet.</p></div>`
            : html`
                <div class="video-grid">
                  ${videos.map((v) => html`
                    <${VideoCard}
                      key=${v.id}
                      video=${v}
                      showDelete=${false}
                      accessState=${accessStateForVideo(v, wallet, accessMap)}
                    />
                  `)}
                </div>
              `}
      </div>
    </div>
  `;
}
