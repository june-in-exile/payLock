const SUI_NETWORK = 'sui:testnet';

let connectedWallet = null;
let connectedAccount = null;
let suiClient = null;
let discoveredWallets = [];
let unsubscribeWalletEvents = null;
let Transaction = null;
let signAndExecuteTransaction = null;
let paywallPackageId = null;
let sealClient = null;
let SealClientClass = null;
let SessionKeyClass = null;
let EncryptedObjectClass = null;
let fromHex = null;
let toHex = null;
let walrusPublisherUrl = null;
let walrusAggregatorUrl = null;
let walrusEpochs = 5;

let onWalletConnectCallback = null;

export function onWalletConnect(callback) {
  onWalletConnectCallback = callback;
}

function findSlushWallet() {
  return discoveredWallets.find(w => w.name.toLowerCase().includes('slush'));
}

async function fetchAppConfig() {
  try {
    const res = await fetch('/api/config');
    if (res.ok) {
      const cfg = await res.json();
      paywallPackageId = cfg.paywall_package_id || null;
      return cfg;
    }
  } catch (_) {}
  return {};
}

export async function initWallet() {
  try {
    const suiVer = '1.45.2';
    const sealVer = '0.10.0';
    const [slushMod, walletMod, suiMod, txMod, walletStdMod, sealMod, utilsMod, configResult] = await Promise.all([
      import('https://esm.sh/@mysten/slush-wallet@1.0.3?bundle'),
      import('https://esm.sh/@wallet-standard/app@1.1.0?bundle'),
      import(`https://esm.sh/@mysten/sui@${suiVer}/client`),
      import(`https://esm.sh/@mysten/sui@${suiVer}/transactions`),
      import('https://esm.sh/@mysten/wallet-standard@0.20.1?bundle'),
      import(`https://esm.sh/@mysten/seal@${sealVer}?deps=@mysten/sui@${suiVer}`),
      import(`https://esm.sh/@mysten/sui@${suiVer}/utils`),
      fetchAppConfig(),
    ]);

    const { registerSlushWallet } = slushMod;
    const { getWallets } = walletMod;
    const { SuiClient, getFullnodeUrl } = suiMod;
    Transaction = txMod.Transaction;
    signAndExecuteTransaction = walletStdMod.signAndExecuteTransaction;

    suiClient = new SuiClient({ url: getFullnodeUrl('testnet') });

    SealClientClass = sealMod.SealClient;
    SessionKeyClass = sealMod.SessionKey;
    EncryptedObjectClass = sealMod.EncryptedObject;
    fromHex = utilsMod.fromHex;
    toHex = utilsMod.toHex;
    walrusPublisherUrl = configResult.walrus_publisher_url || '';
    walrusAggregatorUrl = configResult.walrus_aggregator_url || '';

    sealClient = new SealClientClass({
      suiClient,
      serverConfigs: [
        {
          objectId: '0x73d05d62c18d9374e3ea529e8e0ed6161da1a141a94d3f76ae3fe4e99356db75',
          weight: 1,
        },
        {
          objectId: '0xf5d14a81a982144ae441cd7d64b09027f116a468bd36e7eca494f750591623c8',
          weight: 1,
        },
      ],
      verifyKeyServers: false,
    });

    window._walletStandard = { getWallets, SuiClient, getFullnodeUrl, suiClient };

    const { get, on } = getWallets();
    discoveredWallets = [...get()];

    const unsubRegister = on('register', (...newWallets) => {
      for (const w of newWallets) {
        if (!discoveredWallets.some(existing => existing.name === w.name)) {
          discoveredWallets.push(w);
        }
      }
      const savedAddr = sessionStorage.getItem('paylock_wallet_addr');
      if (savedAddr && !connectedWallet && findSlushWallet()) {
        autoReconnect();
      }
    });

    const unsubUnregister = on('unregister', (...removed) => {
      for (const w of removed) {
        discoveredWallets = discoveredWallets.filter(existing => existing.name !== w.name);
      }
    });

    unsubscribeWalletEvents = () => { unsubRegister(); unsubUnregister(); };

    registerSlushWallet('PayLock Video Streaming', {
      origin: 'https://my.slush.app',
    });

    const savedAddr = sessionStorage.getItem('paylock_wallet_addr');
    if (savedAddr && findSlushWallet()) {
      await autoReconnect();
    }
  } catch (err) {
    console.error('Wallet init failed:', err);
    const btn = document.getElementById('wallet-btn');
    btn.textContent = 'Wallet Unavailable';
    btn.disabled = true;
    btn.style.opacity = '0.5';
    btn.style.cursor = 'not-allowed';
  }
}

async function autoReconnect() {
  try {
    const slush = findSlushWallet();
    if (!slush) return;
    const connectFeature = slush.features['standard:connect'];
    const result = await connectFeature.connect({ silent: true });
    if (result.accounts && result.accounts.length > 0) {
      connectedWallet = slush;
      connectedAccount = result.accounts[0];
      await updateWalletUI();
      if (onWalletConnectCallback) onWalletConnectCallback();
    }
  } catch (_) {
    sessionStorage.removeItem('paylock_wallet_addr');
  }
}

export async function connectWallet() {
  if (connectedWallet) {
    await disconnectWallet();
    return;
  }

  try {
    const slush = findSlushWallet();
    if (!slush) {
      window.open('https://my.slush.app', '_blank');
      return;
    }

    const connectFeature = slush.features['standard:connect'];
    const result = await connectFeature.connect();

    if (result.accounts.length > 0) {
      connectedWallet = slush;
      connectedAccount = result.accounts[0];
      sessionStorage.setItem('paylock_wallet_addr', connectedAccount.address);
      await updateWalletUI();
      if (onWalletConnectCallback) onWalletConnectCallback();
    }
  } catch (err) {
    const btn = document.getElementById('wallet-btn');
    btn.textContent = 'Connection Failed';
    setTimeout(() => { btn.textContent = 'Connect Wallet'; }, 2000);
  }
}

export async function disconnectWallet() {
  if (connectedWallet) {
    try {
      const disconnectFeature = connectedWallet.features['standard:disconnect'];
      if (disconnectFeature) {
        await disconnectFeature.disconnect();
      }
    } catch (_) {}
  }
  connectedWallet = null;
  connectedAccount = null;
  sessionStorage.removeItem('paylock_wallet_addr');

  const btn = document.getElementById('wallet-btn');
  btn.textContent = 'Connect Wallet';
  btn.className = 'wallet-btn connect';
  document.getElementById('wallet-status').style.display = 'none';
}

export async function updateWalletUI() {
  const btn = document.getElementById('wallet-btn');
  const statusEl = document.getElementById('wallet-status');
  const balanceEl = document.getElementById('wallet-balance');
  const addrEl = document.getElementById('wallet-addr');

  if (!connectedAccount) return;

  const addr = connectedAccount.address;
  const shortAddr = addr.slice(0, 6) + '...' + addr.slice(-4);

  btn.textContent = 'Disconnect';
  btn.className = 'wallet-btn connected';

  addrEl.textContent = shortAddr;
  statusEl.style.display = 'flex';

  try {
    const balance = await suiClient.getBalance({ owner: addr });
    const suiAmount = (Number(balance.totalBalance) / 1_000_000_000).toFixed(4);
    balanceEl.textContent = suiAmount + ' SUI';
  } catch (_) {
    balanceEl.textContent = '-- SUI';
  }
}

export async function createVideoOnChain(videoId, price, previewBlobId, fullBlobId) {
  if (!connectedWallet || !connectedAccount) {
    throw new Error('Wallet not connected');
  }
  if (!paywallPackageId) {
    throw new Error('Paywall contract not configured');
  }
  if (!Transaction || !signAndExecuteTransaction) {
    throw new Error('Sui SDK not loaded');
  }

  if (!/^0x[0-9a-fA-F]{64}$/.test(paywallPackageId)) {
    throw new Error('Invalid paywall package ID format');
  }

  const tx = new Transaction();
  tx.moveCall({
    target: paywallPackageId + '::paywall::create_video',
    arguments: [
      tx.pure.u64(price),
      tx.pure.string(previewBlobId),
      tx.pure.string(fullBlobId),
    ],
  });

  const result = await signAndExecuteTransaction(connectedWallet, {
    transaction: tx,
    account: connectedAccount,
    chain: SUI_NETWORK,
  });

  const txResponse = await suiClient.waitForTransaction({
    digest: result.digest,
    options: { showObjectChanges: true },
  });

  const videoType = paywallPackageId + '::paywall::Video';
  const created = (txResponse.objectChanges || []).find(
    c => c.type === 'created' && c.objectType === videoType
  );

  if (!created) {
    throw new Error('Video object not found in transaction result');
  }

  const suiObjectId = created.objectId;

  const res = await fetch('/api/videos/' + encodeURIComponent(videoId) + '/sui-object', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sui_object_id: suiObjectId }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error('Failed to save Sui object ID: ' + (body.error || res.status));
  }

  return suiObjectId;
}

export async function encryptAndPublish(videoId, fileData, price) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!sealClient || !fromHex || !toHex) throw new Error('Seal SDK not loaded');

  // Create Video on-chain with empty blob IDs to get the object ID for Seal encryption
  const suiObjectId = await createVideoOnChain(videoId, price, '', '');

  const nonce = crypto.getRandomValues(new Uint8Array(5));
  const policyObjectBytes = fromHex(suiObjectId.replace(/^0x/, ''));
  const id = toHex(new Uint8Array([...policyObjectBytes, ...nonce]));

  const { encryptedObject: encryptedBytes } = await sealClient.encrypt({
    threshold: 1,
    packageId: paywallPackageId,
    id,
    data: new Uint8Array(fileData),
  });

  const walrusRes = await fetch(walrusPublisherUrl + '/v1/blobs?epochs=' + walrusEpochs, {
    method: 'PUT',
    body: encryptedBytes,
  });
  if (!walrusRes.ok) throw new Error('Walrus upload failed: ' + walrusRes.status);
  const walrusData = await walrusRes.json();
  const fullBlobId = (walrusData.newlyCreated && walrusData.newlyCreated.blobObject && walrusData.newlyCreated.blobObject.blobId)
    || (walrusData.alreadyCertified && walrusData.alreadyCertified.blobId);
  if (!fullBlobId) throw new Error('Failed to get blob ID from Walrus response');

  return { suiObjectId, fullBlobId };
}

export async function updateBlobIds(suiObjectId, videoId, previewBlobId, fullBlobId) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');

  const updateTx = new Transaction();
  updateTx.moveCall({
    target: paywallPackageId + '::paywall::update_preview_blob_id',
    arguments: [
      updateTx.object(suiObjectId),
      updateTx.pure.string(previewBlobId),
    ],
  });
  updateTx.moveCall({
    target: paywallPackageId + '::paywall::update_full_blob_id',
    arguments: [
      updateTx.object(suiObjectId),
      updateTx.pure.string(fullBlobId),
    ],
  });
  await signAndExecuteTransaction(connectedWallet, {
    transaction: updateTx,
    account: connectedAccount,
    chain: SUI_NETWORK,
  });

  const backendRes = await fetch('/api/videos/' + encodeURIComponent(videoId) + '/full-blob', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ full_blob_id: fullBlobId }),
  });
  if (!backendRes.ok) throw new Error('Failed to update backend with full blob ID');
}

export async function findAccessPass(videoSuiObjectId) {
  if (!connectedAccount || !paywallPackageId) return null;
  const accessPassType = paywallPackageId + '::paywall::AccessPass';
  let cursor = null;
  let hasNext = true;
  while (hasNext) {
    const result = await suiClient.getOwnedObjects({
      owner: connectedAccount.address,
      filter: { StructType: accessPassType },
      options: { showContent: true },
      cursor,
    });
    for (const obj of result.data) {
      if (obj.data && obj.data.content && obj.data.content.fields) {
        if (obj.data.content.fields.video_id === videoSuiObjectId) {
          return obj.data.objectId;
        }
      }
    }
    hasNext = result.hasNextPage;
    cursor = result.nextCursor;
  }
  return null;
}

export async function purchaseVideo(video) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!video.sui_object_id) throw new Error('Video not published on-chain');

  const priceInMist = video.price;
  const tx = new Transaction();
  const [paymentCoin] = tx.splitCoins(tx.gas, [priceInMist]);
  tx.moveCall({
    target: paywallPackageId + '::paywall::purchase_and_transfer',
    arguments: [
      tx.object(video.sui_object_id),
      paymentCoin,
    ],
  });
  tx.mergeCoins(tx.gas, [paymentCoin]);

  const result = await signAndExecuteTransaction(connectedWallet, {
    transaction: tx,
    account: connectedAccount,
    chain: SUI_NETWORK,
  });

  const txResponse = await suiClient.waitForTransaction({
    digest: result.digest,
    options: { showObjectChanges: true },
  });

  const accessPassType = paywallPackageId + '::paywall::AccessPass';
  const created = (txResponse.objectChanges || []).find(
    c => c.type === 'created' && c.objectType === accessPassType
  );

  return created ? created.objectId : null;
}

export async function decryptAndPlay(video, knownAccessPassId) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!sealClient || !SessionKeyClass || !EncryptedObjectClass) throw new Error('Seal SDK not loaded');

  const videoEl = document.getElementById('video-player');
  const paywallEl = document.getElementById('paywall-overlay');
  const hintEl = document.getElementById('paywall-hint');

  if (hintEl) hintEl.textContent = 'Decrypting video...';

  const sessionKey = await SessionKeyClass.create({
    address: connectedAccount.address,
    packageId: paywallPackageId,
    ttlMin: 10,
    suiClient,
  });

  const message = sessionKey.getPersonalMessage();
  const signFeature = connectedWallet.features['sui:signPersonalMessage'];
  if (!signFeature) throw new Error('Wallet does not support signPersonalMessage');
  const signResult = await signFeature.signPersonalMessage({
    message,
    account: connectedAccount,
  });
  sessionKey.setPersonalMessageSignature(signResult.signature);

  const encryptedRes = await fetch(video.full_blob_url);
  if (!encryptedRes.ok) throw new Error('Failed to fetch encrypted blob (HTTP ' + encryptedRes.status + ')');
  const encryptedData = new Uint8Array(await encryptedRes.arrayBuffer());
  if (encryptedData.length > 0 && encryptedData[0] === 0x7B) {
    const text = new TextDecoder().decode(encryptedData);
    throw new Error('Walrus returned an error instead of encrypted data: ' + text.slice(0, 200));
  }

  const parsedEncrypted = EncryptedObjectClass.parse(encryptedData);
  const sealId = parsedEncrypted.id;

  const accessPassId = knownAccessPassId || await findAccessPass(video.sui_object_id);
  if (!accessPassId) throw new Error('No AccessPass found for this video');

  const tx = new Transaction();
  tx.moveCall({
    target: paywallPackageId + '::paywall::seal_approve',
    arguments: [
      tx.pure.vector('u8', Array.from(fromHex(toHex(sealId)))),
      tx.object(accessPassId),
      tx.object(video.sui_object_id),
    ],
  });
  const txBytes = await tx.build({ client: suiClient, onlyTransactionKind: true });

  const decryptedBytes = await sealClient.decrypt({
    data: encryptedData,
    sessionKey,
    txBytes,
  });

  if (videoEl.src && videoEl.src.startsWith('blob:')) {
    URL.revokeObjectURL(videoEl.src);
  }
  const blob = new Blob([decryptedBytes], { type: 'video/mp4' });
  const blobUrl = URL.createObjectURL(blob);
  videoEl.onended = null;
  paywallEl.style.display = 'none';
  videoEl.src = blobUrl;
  videoEl.play().catch(() => {});
}

export function isWalletConnected() {
  return !!connectedAccount;
}

export function getWalletAddress() {
  return connectedAccount ? connectedAccount.address : null;
}

export function getPaywallPackageId() {
  return paywallPackageId;
}
