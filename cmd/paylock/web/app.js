import { html, render, useState, useEffect } from './lib.js';
import {
  currentView, walletState, toastState,
  navigate, loadWallet, stageNewFile,
} from './state.js';
import { MyVideosView } from './my-videos-view.js';
import { BrowseView } from './browse-view.js';
import { PlayerView } from './player-view.js';
import { detectVideoDuration } from './upload-section.js';

function Header() {
  const wallet = walletState.value;
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const shortAddr = (addr) => addr ? addr.slice(0, 6) + '...' + addr.slice(-4) : '';

  async function handleConnectClick() {
    const mod = await loadWallet();
    mod.connectWallet();
    setDropdownOpen(false);
  }

  async function handleDisconnect() {
    const mod = await loadWallet();
    mod.disconnectWallet();
    setDropdownOpen(false);
  }

  async function handleSwitch(index) {
    const mod = await loadWallet();
    mod.switchAccount(index);
    setDropdownOpen(false);
  }

  // Close dropdown on outside click
  useEffect(() => {
    if (!dropdownOpen) return;
    function onClickOutside(e) {
      if (!e.target.closest('.wallet-area')) setDropdownOpen(false);
    }
    document.addEventListener('click', onClickOutside);
    return () => document.removeEventListener('click', onClickOutside);
  }, [dropdownOpen]);

  const view = currentView.value;

  if (!wallet.connected) {
    return html`
      <header>
        <div class="logo" onclick=${() => navigate('my-videos')}>
          <span class="logo-text">Pay</span><span class="logo-box">Lock</span>
        </div>
        <nav class="nav-links">
          <a class=${'nav-link' + (view === 'browse' ? ' active' : '')} onclick=${() => navigate('browse')}>Browse</a>
        </nav>
        <div class="wallet-area">
          <button
            class="wallet-btn connect"
            disabled=${!wallet.available}
            style=${!wallet.available ? 'opacity:0.5;cursor:not-allowed' : ''}
            onclick=${handleConnectClick}
          >
            ${!wallet.available ? (wallet.error || 'Wallet Unavailable') : wallet.error ? wallet.error : 'Connect Wallet'}
          </button>
        </div>
      </header>
    `;
  }

  return html`
    <header>
      <div class="logo" onclick=${() => navigate('my-videos')}>
        <span class="logo-text">Pay</span><span class="logo-box">Lock</span>
      </div>
      <nav class="nav-links">
        <a class=${'nav-link' + (view === 'my-videos' ? ' active' : '')} onclick=${() => navigate('my-videos')}>My Videos</a>
        <a class=${'nav-link' + (view === 'browse' ? ' active' : '')} onclick=${() => navigate('browse')}>Browse</a>
      </nav>
      <div class="wallet-area">
        <div class="wallet-info">
          <span class="wallet-dot"></span>
          <span class="wallet-balance">${wallet.balance || '0 SUI'}</span>
        </div>
        <button
          class="wallet-btn connected"
          onclick=${(e) => { e.stopPropagation(); setDropdownOpen(!dropdownOpen); }}
        >
          ${shortAddr(wallet.address)}${' '}
          <span class="wallet-chevron ${dropdownOpen ? 'open' : ''}">▾</span>
        </button>
        ${dropdownOpen && html`
          <div class="wallet-dropdown">
            ${wallet.accounts.map((acct, i) => html`
              <div
                class="wallet-dropdown-item ${i === wallet.activeIndex ? 'active' : ''}"
                key=${acct.address}
                onclick=${() => handleSwitch(i)}
              >
                <span class="wallet-dropdown-addr">${shortAddr(acct.address)}</span>
                ${i === wallet.activeIndex && html`<span class="wallet-dropdown-check">✓</span>`}
              </div>
            `)}
            <div class="wallet-dropdown-divider"></div>
            <div class="wallet-dropdown-item disconnect" onclick=${handleDisconnect}>
              Disconnect
            </div>
          </div>
        `}
      </div>
    </header>
  `;
}

function Toast() {
  const t = toastState.value;
  if (!t) return html`<div class="toast"></div>`;
  return html`
    <div class=${`toast ${t.type}${t.visible ? ' visible' : ''}`}>
      ${t.message}
    </div>
  `;
}

function DragOverlay({ active }) {
  return html`
    <div class=${`upload-overlay${active ? ' active' : ''}`}>
      <div class="upload-overlay-text">Drop MP4 to select</div>
    </div>
  `;
}

function App() {
  const view = currentView.value;
  const [dragging, setDragging] = useState(false);

  // Global drag-and-drop
  useEffect(() => {
    let dragCounter = 0;

    function onDragEnter(e) { e.preventDefault(); dragCounter++; setDragging(true); }
    function onDragLeave(e) { e.preventDefault(); dragCounter--; if (dragCounter <= 0) { dragCounter = 0; setDragging(false); } }
    function onDragOver(e) { e.preventDefault(); }
    function onDrop(e) {
      e.preventDefault();
      dragCounter = 0;
      setDragging(false);
      if (e.dataTransfer.files.length > 0 && currentView.value === 'my-videos') {
        stageNewFile(e.dataTransfer.files[0]);
        detectVideoDuration(e.dataTransfer.files[0]);
      }
    }

    window.addEventListener('dragenter', onDragEnter);
    window.addEventListener('dragleave', onDragLeave);
    window.addEventListener('dragover', onDragOver);
    window.addEventListener('drop', onDrop);

    return () => {
      window.removeEventListener('dragenter', onDragEnter);
      window.removeEventListener('dragleave', onDragLeave);
      window.removeEventListener('dragover', onDragOver);
      window.removeEventListener('drop', onDrop);
    };
  }, []);

  // Client-side router
  useEffect(() => {
    function handleRoute() {
      const path = window.location.pathname;
      if (path.startsWith('/play/')) {
        navigate('player', { id: path.slice(6) }, false);
      } else if (path === '/browse') {
        navigate('browse', {}, false);
      } else {
        navigate('my-videos', {}, false);
      }
    }

    window.addEventListener('popstate', handleRoute);
    handleRoute();
    return () => window.removeEventListener('popstate', handleRoute);
  }, []);

  // Eagerly start wallet loading (non-blocking)
  useEffect(() => { loadWallet().catch(() => {}); }, []);

  return html`
    <${Header} />
    <${DragOverlay} active=${dragging} />
    <main>
      ${view === 'my-videos' && html`<${MyVideosView} />`}
      ${view === 'browse' && html`<${BrowseView} />`}
      ${view === 'player' && html`<${PlayerView} />`}
    </main>
    <${Toast} />
  `;
}

render(html`<${App} />`, document.getElementById('app'));
