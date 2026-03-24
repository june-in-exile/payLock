import { signal, batch } from './lib.js';

// Navigation
export const currentView = signal('videos');
export const viewParams = signal({});
export const navGeneration = signal(0);

// Video data
export const currentVideo = signal(null);

// Wallet
export const walletState = signal({
  connected: false,
  address: null,
  balance: null,
  available: true,
  error: null,
});

// Upload
export const stagedFile = signal(null);
export const uploadState = signal({
  active: false,
  percent: 0,
  text: '',
  step: null,
  showSpinner: false,
  showSteps: false,
});

// Toast
export const toastState = signal(null);

// --- Navigation ---

let pollCleanup = null;

export function setPollCleanup(fn) {
  pollCleanup = fn;
}

export function navigate(view, params = {}, push = true) {
  navGeneration.value = navGeneration.value + 1;

  if (pollCleanup) {
    pollCleanup();
    pollCleanup = null;
  }

  batch(() => {
    currentView.value = view;
    viewParams.value = params;
    currentVideo.value = null;
  });

  if (view === 'videos') {
    if (push) history.pushState({ view }, '', '/');
  } else if (view === 'player' && params.id) {
    if (push) history.pushState({ view, id: params.id }, '', '/play/' + params.id);
  }
}

// --- Toast ---

let toastTimer = null;

export function showToast(type, message, duration = 3000) {
  if (toastTimer) clearTimeout(toastTimer);
  toastState.value = { type, message, visible: true };
  toastTimer = setTimeout(() => {
    toastState.value = { ...toastState.value, visible: false };
    toastTimer = null;
  }, duration);
}

// --- Staged file ---

export function stageNewFile(file) {
  const ext = file.name.toLowerCase().split('.').pop();
  const supported = ['mp4', 'mov', 'webm', 'mkv', 'avi'];
  if (!supported.includes(ext)) {
    showToast('error', 'Unsupported format. Accepted: MP4, MOV, WebM, MKV, AVI.');
    return;
  }
  const prev = stagedFile.value;
  if (prev && prev.objectUrl) URL.revokeObjectURL(prev.objectUrl);
  stagedFile.value = {
    file,
    name: file.name,
    size: file.size,
    objectUrl: URL.createObjectURL(file),
  };
}

export function clearStagedFile() {
  const prev = stagedFile.value;
  if (prev && prev.objectUrl) URL.revokeObjectURL(prev.objectUrl);
  stagedFile.value = null;
}

// --- Wallet loader (lazy) ---

let walletModule = null;
let walletLoading = null;

export async function loadWallet() {
  if (walletModule) return walletModule;
  if (walletLoading) return walletLoading;
  walletLoading = import('./wallet.js')
    .then((mod) => {
      walletModule = mod;
      mod.initWallet();
      return mod;
    })
    .catch((err) => {
      walletLoading = null;
      throw err;
    });
  return walletLoading;
}

export function getWalletModule() {
  return walletModule;
}

// --- Utilities ---

export function formatDate(isoStr) {
  if (!isoStr) return '';
  const d = new Date(isoStr);
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function formatFileSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

export function formatSui(mist) {
  return (mist / 1_000_000_000).toFixed(2);
}
